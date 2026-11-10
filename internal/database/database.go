package database

import (
	"log"

	"github.com/supabase-community/supabase-go"
)

func Connect(url string, key string) *supabase.Client {
	client, err := supabase.NewClient(
		url,
		key,
		&supabase.ClientOptions{},
	)
	if err != nil {
		log.Fatal("Failed to initialize Supabase:", err)
	}
	log.Println("Connected to Supabase")
	return client
}
