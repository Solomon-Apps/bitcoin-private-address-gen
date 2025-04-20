package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/cors"
)

type JobRequest struct {
	StartRange string `json:"startRange"`
	EndRange   string `json:"endRange"`
	Address    string `json:"address"`
}

type Job struct {
	StartRange string   `json:"start_range"`
	EndRange   string   `json:"end_range"`
	Addresses  []string `json:"addresses"`
}

const batchSize = 50000000

func failOnError(err error, msg string) {
	if err != nil {
		log.Printf("%s: %s", msg, err)
	}
}

func sendJobToQueue(ch *amqp.Channel, job Job) error {
	body, err := json.Marshal(job)
	if err != nil {
		return err
	}

	err = ch.PublishWithContext(
		context.Background(),
		"",                    // exchange
		"bitcoin-address-gen", // routing key
		false,                 // mandatory
		false,                 // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		})
	return err
}

func createJobs(startRange, endRange string, address string) ([]Job, error) {
	start, err := strconv.ParseInt(startRange, 10, 64)
	if err != nil {
		return nil, err
	}

	end, err := strconv.ParseInt(endRange, 10, 64)
	if err != nil {
		return nil, err
	}

	var jobs []Job
	for i := start; i < end; i += batchSize {
		batchEnd := i + batchSize
		if batchEnd > end {
			batchEnd = end
		}

		jobs = append(jobs, Job{
			StartRange: fmt.Sprintf("%d", i),
			EndRange:   fmt.Sprintf("%d", batchEnd),
			Addresses:  []string{address},
		})
	}

	return jobs, nil
}

func handleJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var jobReq JobRequest
	if err := json.NewDecoder(r.Body).Decode(&jobReq); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Connect to RabbitMQ
	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		http.Error(w, "RabbitMQ URL not configured", http.StatusInternalServerError)
		return
	}

	conn, err := amqp.Dial(rabbitMQURL)
	if err != nil {
		http.Error(w, "Failed to connect to RabbitMQ", http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		http.Error(w, "Failed to open channel", http.StatusInternalServerError)
		return
	}
	defer ch.Close()

	// Create jobs
	jobs, err := createJobs(jobReq.StartRange, jobReq.EndRange, jobReq.Address)
	if err != nil {
		http.Error(w, "Invalid range values", http.StatusBadRequest)
		return
	}

	// Send jobs to queue
	for _, job := range jobs {
		if err := sendJobToQueue(ch, job); err != nil {
			failOnError(err, "Failed to publish job")
		}
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "Jobs queued successfully",
	})
}

func handleLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "live",
	})
}

func main() {
	// Create CORS handler
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"}, // Allow all origins
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
	})

	// Create router
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleLive)
	mux.HandleFunc("/jobs", handleJobs)

	// Wrap the router with CORS
	handler := c.Handler(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = "5000"
	}

	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}
