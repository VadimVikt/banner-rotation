package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/VadimVikt/banner-rotation/internal/event"
	"github.com/VadimVikt/banner-rotation/internal/repo"
	"github.com/VadimVikt/banner-rotation/internal/service"
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
	_ = svc // wire up handlers in Task 6

	fmt.Println("server initialized successfully")
	fmt.Println("run with --rabbitmq=nop to skip RabbitMQ connection")
}
