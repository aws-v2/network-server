package main

import (
	"encoding/json"
	"log"

	"github.com/nats-io/nats.go"
)

type UserRegisteredEvent struct {
	TenantID   string `json:"tenant_id"`
	TenantName string `json:"tenant_name"`
	Email      string `json:"email"`
}

func main() {
	nc, err := nats.Connect("nats://auth-server:auth-secret@localhost:4222")
	if err != nil {
		log.Fatalf("failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	event := UserRegisteredEvent{
		TenantID:   "tenant-3",
		TenantName: "Cyberdyne",
		Email:      "admin@cyberdyne.com",
	}

	data, _ := json.Marshal(event)
	if err := nc.Publish("dev.auth.v1.user.registered", data); err != nil {
		log.Fatalf("failed to publish event: %v", err)
	}
	log.Printf("Event published for %s\n", event.TenantID)
}
