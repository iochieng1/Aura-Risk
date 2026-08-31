package main

import (
	"log"
	"aurarisk-backend/config"
	"aurarisk-backend/internal/database"
	"aurarisk-backend/internal/handlers"
	"github.com/gin-gonic/gin"
	"github.com/gin-contrib/cors"
)
func main() {
	config.Load()

db := database.Connect()
defer db.Close()

database.Migrate(db)

handlers.SetDatabase(db)

r := gin.Default()

r.Use(cors.New(cors.Config{
	AllowOrigins:     []string{"http://localhost:5173", "https://your-frontend.vercel.app"},
	AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
	AllowHeaders:     []string{"Origin", "Content-Type", "Accept"},
	ExposeHeaders:    []string{"Content-Length"},
	AllowCredentials: true,
}))

r.GET("/health", func(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok"})
})

api := r.Group("/api")
{
	api.GET("/risk", handlers.GetRisk)
	api.GET("/reports", handlers.GetReports)
	api.POST("/reports", handlers.CreateReport)
}

port := config.GetEnv("PORT", "8080")
log.Printf("🚀 AuraRisk backend running on port %s", port)
r.Run(":" + port)
}