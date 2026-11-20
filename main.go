package main

import (
	"log"
	"net/http"
	"os"

	"app/internal/config"
	"app/internal/database"
	"app/internal/server"
)

func main() {
	cfg := config.LoadConfig()

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Urutan yang benar: cfg dulu, baru db
	r := server.SetupRouter(cfg, db)

	port := os.Getenv("PORT")
	if port == "" {
		port = cfg.AppPort
	}
	if port == "" {
		port = "8081"
	}

	log.Printf("Server running on port %s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}