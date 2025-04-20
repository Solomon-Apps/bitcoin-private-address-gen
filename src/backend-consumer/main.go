package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Job struct {
	StartRange string   `json:"start_range"`
	EndRange   string   `json:"end_range"`
	Addresses  []string `json:"addresses"`
}

func failOnError(err error, msg string) {
	if err != nil {
		fmt.Printf("%s: %s\n", msg, err)
		os.Exit(1)
	}
}

func writeAddressesToFile(addresses []string) error {
	file, err := os.Create("addresses.txt")
	if err != nil {
		return err
	}
	defer file.Close()

	for _, addr := range addresses {
		_, err := file.WriteString(addr + "\n")
		if err != nil {
			return err
		}
	}
	return nil
}

func runKeyhunt(job Job, wg *sync.WaitGroup, ch *amqp.Channel) {
	defer wg.Done()

	// Write addresses to file
	err := writeAddressesToFile(job.Addresses)
	if err != nil {
		fmt.Printf("Error writing addresses: %v\n", err)
		return
	}

	// Run keyhunt command
	cmd := exec.Command("/usr/local/bin/keyhunt",
		"-r", fmt.Sprintf("%s:%s", job.StartRange, job.EndRange),
		"-l", "compress",
		"-k", "250",
		"-f", "./addresses.txt",
		"-t", "4",
		"-B", "sequential")

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error running keyhunt: %v\n", err)
		return
	}

	// Check if private key was found
	if strings.Contains(string(output), "Private Key") {
		// Publish to found queue
		err = ch.PublishWithContext(context.Background(),
			"",                           // exchange
			"bitcoin-addresss-gen-found", // routing key
			false,                        // mandatory
			false,                        // immediate
			amqp.Publishing{
				ContentType: "text/plain",
				Body:        output,
			})
		if err != nil {
			fmt.Printf("Error publishing to found queue: %v\n", err)
		}
	}
}

func main() {
	// Get RabbitMQ URL from environment variable
	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		failOnError(fmt.Errorf("RABBITMQ_URL environment variable not set"), "Failed to get RabbitMQ URL")
	}

	// Connect to RabbitMQ
	conn, err := amqp.Dial(rabbitMQURL)
	failOnError(err, "Failed to connect to RabbitMQ")
	defer conn.Close()

	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()

	// Declare queues
	q, err := ch.QueueDeclare(
		"bitcoin-address-gen", // queue name
		true,                  // durable
		false,                 // delete when unused
		false,                 // exclusive
		false,                 // no-wait
		nil,                   // arguments
	)
	failOnError(err, "Failed to declare a queue")

	// Set prefetch count to 2
	err = ch.Qos(
		2,     // prefetch count
		0,     // prefetch size
		false, // global
	)
	failOnError(err, "Failed to set QoS")

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	failOnError(err, "Failed to register a consumer")

	var wg sync.WaitGroup
	activeJobs := 0

	for msg := range msgs {
		// Parse job from message
		var job Job
		err := json.Unmarshal(msg.Body, &job)
		if err != nil {
			fmt.Printf("Error parsing job: %v\n", err)
			msg.Nack(false, false) // Reject message without requeue
			continue
		}

		wg.Add(1)
		activeJobs++
		go runKeyhunt(job, &wg, ch)

		// If we have less than 2 active jobs, acknowledge the message
		if activeJobs < 2 {
			msg.Ack(false)
		}

		// Wait for at least one job to complete before getting another
		if activeJobs >= 2 {
			wg.Wait()
			activeJobs = 0
		}
	}
}
