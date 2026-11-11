package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/jijahh/cookies4cuties_server_golang/internal/config"
	"github.com/jijahh/cookies4cuties_server_golang/internal/database"
	"github.com/jijahh/cookies4cuties_server_golang/internal/routes"
)

func main() {
	cfg := config.LoadConfig()
	client := database.Connect(
		cfg.SUPABASE_URL,
		cfg.SUPABASE_KEY,
	)
	router := gin.Default()
	routes.SetupRoutes(router, client, cfg)
	if os.Getenv("VERCEL") != "" {
		gin.SetMode(gin.ReleaseMode)
	}
	log.Println("Server running on port:", cfg.PORT)

	if err := router.Run(":" + cfg.PORT); err != nil {
		log.Fatal(err)
	}
}
