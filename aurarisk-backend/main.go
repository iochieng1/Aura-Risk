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
	"aurarisk-backend/internal/services"
	"aurarisk-backend/internal/storage"
	"aurarisk-backend/internal/workers"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

const maxJSONBodyBytes = 64 << 10

func main() {
	config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db := database.Connect()
	defer db.Close()

	database.Migrate(db)

	handlers.SetDatabase(db)

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

	if interval := config.GetDuration("NOTIFIER_INTERVAL", 15*time.Minute); interval > 0 {
		notifier := &workers.Notifier{
			DB:       db,
			Push:     services.NewExpoPushClient(config.GetEnv("EXPO_PUSH_URL", ""), config.GetEnv("EXPO_ACCESS_TOKEN", "")),
			Assess:   services.GenerateRiskAssessment,
			Interval: interval,
		}
		go notifier.Run(ctx)
		log.Printf("✅ Risk notifier running every %s", interval)
	}

	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "https://your-frontend.vercel.app"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

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

	<-ctx.Done()
	log.Println("Shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Graceful shutdown failed: %v", err)
	}
}
