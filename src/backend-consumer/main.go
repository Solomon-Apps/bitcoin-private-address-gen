package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

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

var activeProcesses int
var processMutex sync.Mutex

func runKeyhunt(job Job, wg *sync.WaitGroup, ch *amqp.Channel) {
	defer wg.Done()

	// Increment active process counter
	processMutex.Lock()
	activeProcesses++
	processMutex.Unlock()

	// Print working directory and keyhunt path
	wd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting working directory: %v\n", err)
	} else {
		fmt.Printf("Current working directory: %s\n", wd)
	}

	// Use absolute path for keyhunt
	keyhuntPath := "./keyhunt"

	// Check if keyhunt exists and is executable
	if fileInfo, err := os.Stat(keyhuntPath); err != nil {
		fmt.Printf("Keyhunt binary not found at %s: %v\n", keyhuntPath, err)
		return
	} else {
		fmt.Printf("Keyhunt binary found. Size: %d bytes, Mode: %v\n", fileInfo.Size(), fileInfo.Mode())
		if fileInfo.Mode()&0111 == 0 {
			fmt.Printf("Keyhunt binary is not executable. Current permissions: %v\n", fileInfo.Mode())
			return
		}
	}

	// Write addresses to file
	err = writeAddressesToFile(job.Addresses)
	if err != nil {
		fmt.Printf("Error writing addresses: %v\n", err)
		return
	}

	// Run keyhunt command
	cmd := exec.Command(keyhuntPath,
		"-r", fmt.Sprintf("%s:%s", job.StartRange, job.EndRange),
		"-l", "compress",
		"-k", "250",
		"-f", "./addresses.txt",
		"-t", "4",
		"-B", "sequential")

	// Print the full command
	fmt.Printf("Executing command: %v\n", cmd.Args)

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error running keyhunt: %v\n", err)
		fmt.Printf("Command output: %s\n", string(output))
		return
	}

	// Print the output
	fmt.Printf("Keyhunt output: %s\n", string(output))

	// Check for KEYFOUNDKEYFOUND.txt file
	keyFoundFile := "KEYFOUNDKEYFOUND.txt"
	if _, err := os.Stat(keyFoundFile); err == nil {
		// File exists, read its contents
		content, err := os.ReadFile(keyFoundFile)
		if err != nil {
			fmt.Printf("Error reading key found file: %v\n", err)
			return
		}

		// Publish to found queue
		err = ch.PublishWithContext(context.Background(),
			"",                           // exchange
			"bitcoin-addresss-gen-found", // routing key
			false,                        // mandatory
			false,                        // immediate
			amqp.Publishing{
				ContentType: "text/plain",
				Body:        content,
			})
		if err != nil {
			fmt.Printf("Error publishing to found queue: %v\n", err)
			return
		}

		fmt.Printf("Published to found queue: %s\n", string(content))

		// Delete the file
		err = os.Remove(keyFoundFile)
		if err != nil {
			fmt.Printf("Error deleting key found file: %v\n", err)
		}
	}

	// Decrement the active process counter once the keyhunt process is complete
	processMutex.Lock()
	activeProcesses--
	processMutex.Unlock()
}

func main() {
	// Print working directory at startup
	wd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting working directory: %v\n", err)
	} else {
		fmt.Printf("Starting in directory: %s\n", wd)
	}

	// Check keyhunt binary
	keyhuntPath := "./keyhunt"
	if fileInfo, err := os.Stat(keyhuntPath); err != nil {
		fmt.Printf("Keyhunt binary not found at startup: %v\n", err)
	} else {
		fmt.Printf("Keyhunt binary found at startup. Size: %d bytes, Mode: %v\n", fileInfo.Size(), fileInfo.Mode())
	}

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

	for msg := range msgs {
		// Parse job from message
		var job Job
		err := json.Unmarshal(msg.Body, &job)
		if err != nil {
			fmt.Printf("Error parsing job: %v\n", err)
			msg.Nack(false, false) // Reject message without requeue
			continue
		}

		// Acknowledge the message as soon as we start processing it
		msg.Ack(false)

		// Wait until there are no active keyhunt processes before starting a new one
		processMutex.Lock()
		for activeProcesses >= 2 {
			processMutex.Unlock()
			// Wait for a short time before checking again
			time.Sleep(100 * time.Millisecond)
			processMutex.Lock()
		}

		// Start the job
		wg.Add(1)
		go runKeyhunt(job, &wg, ch)

		// Unlock mutex after starting job
		processMutex.Unlock()
	}

	// Wait for all jobs to complete before exiting
	wg.Wait()
}
