package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"aurarisk-backend/config"
	"aurarisk-backend/internal/database"
	"aurarisk-backend/internal/handlers"
	"aurarisk-backend/internal/metrics"
	"aurarisk-backend/internal/services"
	"aurarisk-backend/internal/storage"
	"aurarisk-backend/internal/workers"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

const maxJSONBodyBytes = 64 << 10

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	loadCtx, cancelLoad := context.WithTimeout(ctx, 30*time.Second)
	err := config.Load(loadCtx)
	cancelLoad()
	if err != nil {
		log.Fatalf("Startup aborted: %v", err)
	}

	db := database.Connect()
	defer db.Close()

	// A bounded pool keeps load from exhausting Postgres connections and makes
	// pool saturation measurable (see go_sql_* metrics).
	db.SetMaxOpenConns(config.GetInt("DB_MAX_OPEN_CONNS", 25))
	db.SetMaxIdleConns(config.GetInt("DB_MAX_IDLE_CONNS", 10))
	db.SetConnMaxIdleTime(5 * time.Minute)
	metrics.RegisterDB(db, "aurarisk")

	database.Migrate(db)

	handlers.SetDatabase(db)

	staleFor := config.GetDuration("WEATHER_STALE_MAX", 3*time.Hour)
	if staleFor == 0 {
		staleFor = -1 // 0 in config means never serve stale data
	}
	openMeteo := &services.OpenMeteoClient{
		URL:    config.GetEnv("OPEN_METEO_URL", ""),
		APIKey: config.GetEnv("OPEN_METEO_API_KEY", ""),
	}
	services.SetWeatherService(services.NewWeatherService(openMeteo, services.WeatherConfig{
		FreshFor:             config.GetDuration("WEATHER_CACHE_TTL", 15*time.Minute),
		StaleFor:             staleFor,
		MaxRequestsPerMinute: config.GetInt("WEATHER_MAX_REQUESTS_PER_MINUTE", 300),
	}))
	if openMeteo.APIKey == "" && config.IsProduction() {
		log.Println("⚠️ OPEN_METEO_API_KEY is not set: using the free Open-Meteo API, which is for non-commercial use only")
	}

	moderators, err := config.ModeratorTokens()
	if err != nil {
		log.Fatalf("Startup aborted: %v", err)
	}
	handlers.SetModerators(moderators)
	if len(moderators) == 0 {
		log.Println("⚠️ MODERATOR_TOKENS is not set: the moderation API is disabled")
	}

	var store storage.ObjectStore
	if bucket := config.GetEnv("S3_BUCKET", ""); bucket != "" {
		s3Store, err := storage.NewS3Store(ctx, storage.S3Config{
			Bucket:         bucket,
			Region:         config.GetEnv("S3_REGION", "us-east-1"),
			Endpoint:       config.GetEnv("S3_ENDPOINT", ""),
			PublicEndpoint: config.GetEnv("S3_PUBLIC_ENDPOINT", ""),
			ForcePathStyle: config.GetBool("S3_FORCE_PATH_STYLE", false),
		})
		if err != nil {
			log.Fatalf("Failed to configure object storage: %v", err)
		}
		store = s3Store
		handlers.SetPhotoStore(store)
		log.Printf("✅ Object storage configured (bucket %s)", bucket)
	} else {
		log.Println("⚠️ S3_BUCKET is not set: photo uploads are disabled")
	}

	if store != nil {
		processor := &workers.PhotoProcessor{DB: db, Store: store, Interval: 5 * time.Second}
		if addr := config.GetEnv("CLAMD_ADDR", ""); addr != "" {
			processor.Scanner = services.NewClamdScanner(addr)
		}
		go processor.Run(ctx)
	}

	if interval := config.GetDuration("NOTIFIER_INTERVAL", config.DefaultNotifierInterval); interval > 0 {
		notifier := &workers.Notifier{
			DB:       db,
			Push:     services.NewExpoPushClient(config.GetEnv("EXPO_PUSH_URL", ""), config.GetEnv("EXPO_ACCESS_TOKEN", "")),
			Assess:   services.GenerateRiskAssessment,
			Interval: interval,
		}
		go notifier.Run(ctx)
		log.Printf("✅ Risk notifier running every %s", interval)
	}

	if interval := config.GetDuration("REPORT_VERIFIER_INTERVAL", 2*time.Minute); interval > 0 {
		go (&workers.ReportVerifier{DB: db, Interval: interval}).Run(ctx)
		log.Printf("✅ Report verifier running every %s", interval)
	}

	if interval := config.GetDuration("RETENTION_INTERVAL", time.Hour); interval > 0 {
		policy := workers.DefaultRetentionPolicy()
		days := func(key string, fallback time.Duration) time.Duration {
			return time.Duration(config.GetInt(key, int(fallback/(24*time.Hour)))) * 24 * time.Hour
		}
		policy.Rejected = days("RETENTION_REJECTED_DAYS", policy.Rejected)
		policy.Duplicate = days("RETENTION_DUPLICATE_DAYS", policy.Duplicate)
		policy.Pending = days("RETENTION_PENDING_DAYS", policy.Pending)
		policy.Verified = days("RETENTION_VERIFIED_DAYS", policy.Verified)
		policy.FailedPhoto = days("RETENTION_FAILED_PHOTO_DAYS", policy.FailedPhoto)
		go (&workers.RetentionCleaner{DB: db, Store: store, Policy: policy, Interval: interval}).Run(ctx)
		log.Printf("✅ Retention cleanup running every %s", interval)
	}

	r := gin.Default()
	r.Use(metrics.Middleware())

	// Web app origins allowed to call the API from a browser (validated in
	// config). Empty disables CORS, for when the app and API share an origin.
	if origins := config.CORSOrigins(); len(origins) > 0 {
		r.Use(cors.New(cors.Config{
			AllowOrigins:     origins,
			AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
			ExposeHeaders:    []string{"Content-Length"},
			AllowCredentials: true,
		}))
	}

	r.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxJSONBodyBytes)
		c.Next()
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	api := r.Group("/api")
	{
		api.GET("/risk", handlers.GetRisk)
		api.GET("/reports", handlers.GetReports)
		api.POST("/reports", handlers.OptionalAuth(), handlers.CreateReport)

		api.POST("/auth/device", handlers.RegisterDevice)
		api.POST("/auth/refresh", handlers.RefreshTokens)

		authed := api.Group("", handlers.RequireAuth())
		{
			authed.POST("/auth/logout", handlers.Logout)
			authed.DELETE("/account", handlers.DeleteAccount)

			authed.PUT("/devices/me/push", handlers.RegisterPushToken)
			authed.DELETE("/devices/me/push", handlers.UnregisterPushToken)

			authed.GET("/subscriptions", handlers.ListSubscriptions)
			authed.POST("/subscriptions", handlers.CreateSubscription)
			authed.PUT("/subscriptions/:id", handlers.UpdateSubscription)
			authed.DELETE("/subscriptions/:id", handlers.DeleteSubscription)

			authed.POST("/reports/:id/photos", handlers.CreatePhotoUpload)
			authed.POST("/photos/:id/complete", handlers.CompletePhotoUpload)
			authed.GET("/photos/:id", handlers.GetPhoto)
		}

		moderation := api.Group("/moderation", handlers.RequireModerator())
		{
			moderation.GET("/reports", handlers.ListModerationQueue)
			moderation.POST("/reports/:id", handlers.ModerateReport)
			moderation.GET("/reports/:id/events", handlers.ListModerationEvents)
		}
	}

	port := config.GetEnv("PORT", "8080")
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("🚀 AuraRisk backend running on port %s", port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Metrics get their own listener so /metrics is never exposed on the public
	// API port. Bind it to a private interface in production.
	var metricsServer *http.Server
	if addr := config.GetEnv("METRICS_ADDR", "localhost:9464"); addr != "" {
		mux := http.NewServeMux()
		mux.Handle("/metrics", metrics.Handler())
		metricsServer = &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
		go func() {
			log.Printf("📈 Metrics available at http://%s/metrics", addr)
			if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("Metrics server failed: %v", err)
			}
		}()
	}

	<-ctx.Done()
	log.Println("Shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Graceful shutdown failed: %v", err)
	}
	if metricsServer != nil {
		_ = metricsServer.Shutdown(shutdownCtx)
	}
}
