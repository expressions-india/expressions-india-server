package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	if _, statErr := os.Stat(".env"); statErr == nil {
		if err := godotenv.Load(); err != nil {
			log.Fatal("Found .env file but could not parse it: ", err)
		}
	}

	if secret := os.Getenv("JWT_SECRET"); len(secret) < 32 {
		log.Fatal("JWT_SECRET env var is missing or shorter than 32 chars")
	}
	if os.Getenv("APP_ENV") == "production" && os.Getenv("ALLOWED_ORIGINS") == "" {
		log.Fatal("ALLOWED_ORIGINS must be set in production")
	}

	server := initServer()
	server.SetupRoutes()
	if err := SeedAdminUser(server); err != nil {
		log.Fatal("Failed to seed admin user", err.Error(), ".\nSkipping")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           server.r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()
	log.Printf("server listening on :%s", port)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
