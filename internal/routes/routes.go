package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/jijahh/cookies4cuties_server_golang/internal/handlers"
	"github.com/jijahh/cookies4cuties_server_golang/internal/middleware"
	"github.com/jijahh/cookies4cuties_server_golang/internal/services"
	"github.com/supabase-community/supabase-go"
)

func SetupRoutes(router *gin.Engine, client *supabase.Client, adminKey string, orderBucket string, productBucket string, maxUploadSizeMB int, emailService *services.EmailService) {
	orderHandler := handlers.NewOrderHandler(client, orderBucket, int64(maxUploadSizeMB), emailService)
	productHandler := handlers.NewProductHandler(client, productBucket, int64(maxUploadSizeMB))
	adminAuth := middleware.RequireAdmin(adminKey)
	admin := router.Group("/admin")
	admin.Use(adminAuth)
	{
		admin.GET("orders", orderHandler.GetOrders)
		admin.POST("products", productHandler.CreateProduct)
		admin.PATCH("/orders/:order_id", orderHandler.UpdateOrder)
	}
	router.POST("orders", orderHandler.CreateOrder)
}
