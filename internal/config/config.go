package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	PORT                             string
	SUPABASE_URL                     string
	SUPABASE_KEY                     string
	ADMIN_API_KEY                    string
	SMTP_SERVER                      string
	SMTP_PORT                        string
	SENDER_EMAIL                     string
	SENDER_PASSWORD                  string
	SUPABASE_STORAGE_BUCKET_ORDERS   string
	SUPABASE_STORAGE_BUCKET_PRODUCTS string
	MAX_UPLOAD_SIZE                  int
	QSTASH_TOKEN                     string
	QSTASH_CURRENT_SIGNING_KEY       string
	QSTASH_NEXT_SIGNING_KEY          string
	EMAIL_WORKER_URL                 string
}

func LoadConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using system environment variables")
	}
	port := "8000"
	if value, ok := os.LookupEnv("PORT"); ok {
		port = value
	}
	bucket_orders := "payment-proofs"
	bucket_products := "product-images"
	maxUploadSizeMB := 5

	return &Config{
		PORT:                             port,
		SUPABASE_URL:                     os.Getenv("SUPABASE_URL"),
		SUPABASE_KEY:                     os.Getenv("SUPABASE_KEY"),
		ADMIN_API_KEY:                    os.Getenv("ADMIN_API_KEY"),
		SMTP_SERVER:                      os.Getenv("SMTP_SERVER"),
		SMTP_PORT:                        os.Getenv("SMTP_PORT"),
		SENDER_EMAIL:                     os.Getenv("SENDER_EMAIL"),
		SENDER_PASSWORD:                  os.Getenv("SENDER_PASSWORD"),
		SUPABASE_STORAGE_BUCKET_ORDERS:   bucket_orders,
		SUPABASE_STORAGE_BUCKET_PRODUCTS: bucket_products,
		MAX_UPLOAD_SIZE:                  maxUploadSizeMB,
		QSTASH_TOKEN:                     os.Getenv("QSTASH_TOKEN"),
		QSTASH_CURRENT_SIGNING_KEY:       os.Getenv("QSTASH_CURRENT_SIGNING_KEY"),
		QSTASH_NEXT_SIGNING_KEY:          os.Getenv("QSTASH_NEXT_SIGNING_KEY"),
		EMAIL_WORKER_URL:                 os.Getenv("EMAIL_WORKER_URL"),
	}
}
