package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

type UserRegisteredEvent struct {
	CorrelationID string `json:"correlation_id"`
	TenantID      string `json:"tenant_id"`
	TenantName    string `json:"tenant_name"`
	Email         string `json:"email"`
}

func main() {
	url := "nats://auth-server:auth-secret@localhost:4222"
	nc, err := nats.Connect(url)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	event := UserRegisteredEvent{
		CorrelationID: uuid.New().String(),
		TenantID:      "tenant-002",
		TenantName:    "Boutique Tenant",
		Email:         "admin@boutique.com",
	}

	data, _ := json.Marshal(event)
	err = nc.Publish("dev.auth.v1.user.registered", data)
	if err != nil {
		log.Fatalf("Failed to publish: %v", err)
	}

	fmt.Printf("Successfully published event: %s\n", string(data))
	time.Sleep(500 * time.Millisecond) // Give NATS time to flush
}
