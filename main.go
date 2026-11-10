package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/jijahh/cookies4cuties_server_golang/internal/config"
	"github.com/jijahh/cookies4cuties_server_golang/internal/database"
	"github.com/jijahh/cookies4cuties_server_golang/internal/routes"
	"github.com/jijahh/cookies4cuties_server_golang/internal/services"
)

func main() {
	cfg := config.LoadConfig()
	emailService := services.NewEmailService(
		cfg.SMTP_SERVER,
		cfg.SMTP_PORT,
		cfg.SENDER_EMAIL,
		cfg.SENDER_PASSWORD,
	)
	client := database.Connect(
		cfg.SUPABASE_URL,
		cfg.SUPABASE_KEY,
	)
	router := gin.Default()
	routes.SetupRoutes(router, client, cfg.ADMIN_API_KEY, cfg.SUPABASE_STORAGE_BUCKET_ORDERS, cfg.SUPABASE_STORAGE_BUCKET_PRODUCTS, cfg.MAX_UPLOAD_SIZE, emailService)
	if os.Getenv("VERCEL") != "" {
		gin.SetMode(gin.ReleaseMode)
	}
	log.Println("Server running on port:", cfg.PORT)

	if err := router.Run(":" + cfg.PORT); err != nil {
		log.Fatal(err)
	}
}
