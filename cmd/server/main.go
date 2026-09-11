package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/VadimVikt/banner-rotation/internal/event"
	"github.com/VadimVikt/banner-rotation/internal/handler"
	"github.com/VadimVikt/banner-rotation/internal/repo"
	"github.com/VadimVikt/banner-rotation/internal/service"
	"github.com/go-chi/chi/v5"
)

func main() {
	// 1. Initialize configuration
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dbPath := flag.String("db", "banner_rotation.db", "SQLite database path")
	rabbitmqURL := flag.String("rabbitmq", os.Getenv("RABBITMQ_URL"), "RabbitMQ URL (use 'nop' to skip)")
	flag.Parse()

	if *rabbitmqURL == "" {
		*rabbitmqURL = "amqp://guest:guest@localhost:5672/"
	}

	log.Printf("starting server on %s", *addr)
	log.Printf("database: %s", *dbPath)
	log.Printf("rabbitmq: %s", *rabbitmqURL)

	// 2. Connect to database (SQLite)
	r, err := repo.NewRepo(*dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer r.Close()

	// 3. Connect to RabbitMQ (use NOPublisher when URL is "nop")
	var pub event.Publisher
	if *rabbitmqURL == "nop" {
		pub = event.NOPublisher{}
		log.Println("using NOPublisher (events will be discarded)")
	} else {
		rabbitPub, err := event.NewRabbitMQPublisher(*rabbitmqURL)
		if err != nil {
			log.Fatalf("failed to connect to RabbitMQ: %v", err)
		}
		defer rabbitPub.Close()
		pub = rabbitPub
	}

	// 4. Create service
	svc := service.NewService(r, pub)

	// 5. Create handlers and register routes
	h := handler.NewHandlers(svc)
	router := chi.NewMux()
	h.RegisterRoutes(router)

	// 6. Start HTTP server
	srv := &http.Server{
		Addr:         *addr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	// Graceful shutdown
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	fmt.Printf("server started on %s\n", *addr)
	fmt.Println("run with --rabbitmq=nop to skip RabbitMQ connection")

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down server...")

	// Shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}
	log.Println("server stopped")
}
