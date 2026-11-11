package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/jijahh/cookies4cuties_server_golang/internal/config"
	"github.com/jijahh/cookies4cuties_server_golang/internal/handlers"
	"github.com/jijahh/cookies4cuties_server_golang/internal/middleware"
	"github.com/jijahh/cookies4cuties_server_golang/internal/services"
	"github.com/supabase-community/supabase-go"
)

func SetupRoutes(router *gin.Engine, client *supabase.Client, config *config.Config) {
	emailService := services.NewEmailService(
		config.SMTP_SERVER,
		config.SMTP_PORT,
		config.SENDER_EMAIL,
		config.SENDER_PASSWORD,
	)
	emailQueue := services.NewEmailQueue(
		config.QSTASH_TOKEN,
		config.EMAIL_WORKER_URL,
	)
	emailHandler := handlers.NewEmailHandler(
		emailService,
		config.QSTASH_CURRENT_SIGNING_KEY,
		config.QSTASH_NEXT_SIGNING_KEY,
		config.EMAIL_WORKER_URL,
	)
	orderHandler := handlers.NewOrderHandler(
		client,
		config.SUPABASE_STORAGE_BUCKET_ORDERS,
		int64(config.MAX_UPLOAD_SIZE),
		emailService,
		emailQueue,
	)
	productHandler := handlers.NewProductHandler(
		client,
		config.SUPABASE_STORAGE_BUCKET_PRODUCTS,
		int64(config.MAX_UPLOAD_SIZE),
	)
	adminAuth := middleware.RequireAdmin(config.ADMIN_API_KEY)
	admin := router.Group("/admin")
	admin.Use(adminAuth)
	{
		admin.GET("orders", orderHandler.GetOrders)
		admin.POST("products", productHandler.CreateProduct)
		admin.PATCH("/orders/:order_id", orderHandler.UpdateOrder)
	}
	router.POST("orders", orderHandler.CreateOrder)
	router.POST("api/tasks/send-order-email", emailHandler.SendOrderEmail)
}
