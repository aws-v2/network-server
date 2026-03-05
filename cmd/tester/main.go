package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// Domain models (duplicated here for simplicity/standalone utility)
type UserRegisteredEvent struct {
	CorrelationID string `json:"correlation_id"`
	TenantID      string `json:"tenant_id"`
	TenantName    string `json:"tenant_name"`
	Email         string `json:"email"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: tester <subcommand> [flags]")
		fmt.Println("Subcommands: register, batch, eip-assoc, eip-disassoc")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "register":
		registerCmd()
	case "batch":
		batchCmd()
	case "eip-assoc":
		eipAssocCmd()
	case "eip-disassoc":
		eipDisassocCmd()
	case "attach-subnet":
		attachSubnetCmd()
	case "resolve-resource":
		resolveResourceCmd()
	case "list-resources":
		listResourcesCmd()
	case "resolve-default":
		resolveDefaultCmd()
	case "register-compute":
		registerComputeCmd()
	case "compute-health":
		computeHealthCmd()
	case "deregister-compute":
		deregisterComputeCmd()
	case "list-compute":
		listComputeCmd()
	case "compute-lifecycle":
		computeLifecycleCmd()
	case "route-compute":
		routeComputeCmd()
	default:
		fmt.Printf("Unknown subcommand: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func registerCmd() {
	fs := flag.NewFlagSet("register", flag.ExitOnError)
	tenantID := fs.String("tenant", "tenant-001", "Tenant ID")
	tenantName := fs.String("name", "Test Tenant", "Tenant Name")
	natsURL := fs.String("nats", "nats://auth-server:auth-secret@localhost:4222", "NATS URL")
	fs.Parse(os.Args[2:])

	nc, err := nats.Connect(*natsURL)
	if err != nil {
		log.Fatalf("NATS connect failed: %v", err)
	}
	defer nc.Close()

	publishEvent(nc, *tenantID, *tenantName)
}

func batchCmd() {
	fs := flag.NewFlagSet("batch", flag.ExitOnError)
	count := fs.Int("count", 5, "Number of events to publish")
	concurrent := fs.Bool("concurrent", true, "Publish concurrently")
	natsURL := fs.String("nats", "nats://auth-server:auth-secret@localhost:4222", "NATS URL")
	fs.Parse(os.Args[2:])

	nc, err := nats.Connect(*natsURL)
	if err != nil {
		log.Fatalf("NATS connect failed: %v", err)
	}
	defer nc.Close()

	var wg sync.WaitGroup
	for i := 1; i <= *count; i++ {
		tID := fmt.Sprintf("tenant-%03d", i)
		tName := fmt.Sprintf("Tenant %d", i)

		if *concurrent {
			wg.Add(1)
			go func(id, name string) {
				defer wg.Done()
				publishEvent(nc, id, name)
			}(tID, tName)
		} else {
			publishEvent(nc, tID, tName)
		}
	}
	wg.Wait()
}

func eipAssocCmd() {
	fs := flag.NewFlagSet("eip-assoc", flag.ExitOnError)
	apiURL := fs.String("api", "http://localhost:8080", "API URL")
	eipID := fs.String("eip", "", "EIP ID")
	instID := fs.String("instance", "", "Instance ID")
	privIP := fs.String("ip", "", "Private IP")
	fs.Parse(os.Args[2:])

	if *eipID == "" || *instID == "" || *privIP == "" {
		log.Fatal("eip, instance, and ip are required")
	}

	url := fmt.Sprintf("%s/v1/eip/%s/associate", *apiURL, *eipID)
	body, _ := json.Marshal(map[string]string{
		"instance_id": *instID,
		"private_ip":  *privIP,
	})

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	fmt.Printf("Association response: %s\n", resp.Status)
}

func eipDisassocCmd() {
	fs := flag.NewFlagSet("eip-disassoc", flag.ExitOnError)
	apiURL := fs.String("api", "http://localhost:8080", "API URL")
	eipID := fs.String("eip", "", "EIP ID")
	fs.Parse(os.Args[2:])

	if *eipID == "" {
		log.Fatal("eip is required")
	}

	url := fmt.Sprintf("%s/v1/eip/%s/disassociate", *apiURL, *eipID)
	req, _ := http.NewRequest(http.MethodPost, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	fmt.Printf("Disassociation response: %s\n", resp.Status)
}

func publishEvent(nc *nats.Conn, tenantID, tenantName string) {
	event := UserRegisteredEvent{
		CorrelationID: uuid.New().String(),
		TenantID:      tenantID,
		TenantName:    tenantName,
		Email:         fmt.Sprintf("admin@%s.com", tenantID),
	}

	data, _ := json.Marshal(event)
	err := nc.Publish("dev.auth.v1.user.registered", data)
	if err != nil {
		log.Printf("[%s] Publish failed: %v", tenantID, err)
		return
	}
	fmt.Printf("[%s] Published event: %s\n", tenantID, string(data))
}

func attachSubnetCmd() {
	fs := flag.NewFlagSet("attach-subnet", flag.ExitOnError)
	natsURL := fs.String("nats", "nats://auth-server:auth-secret@localhost:4222", "NATS URL")
	tenantID := fs.String("tenant", "tenant-001", "Tenant ID")
	arn := fs.String("arn", "", "Resource ARN")
	vpcID := fs.String("vpc", "", "VPC ID")
	subnetID := fs.String("subnet", "", "Subnet ID")
	fs.Parse(os.Args[2:])

	if *arn == "" || *vpcID == "" || *subnetID == "" {
		log.Fatal("arn, vpc, and subnet are required")
	}

	nc, _ := nats.Connect(*natsURL)
	defer nc.Close()

	req := map[string]interface{}{
		"correlation_id":   uuid.New().String(),
		"tenant_id":        *tenantID,
		"resource_arn":     *arn,
		"target_vpc_id":    *vpcID,
		"target_subnet_id": *subnetID,
	}
	data, _ := json.Marshal(req)

	msg, err := nc.Request("dev.network.v1.resource.attach", data, nats.DefaultTimeout)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	fmt.Printf("Attach Response: %s\n", string(msg.Data))
}

func resolveResourceCmd() {
	fs := flag.NewFlagSet("resolve-resource", flag.ExitOnError)
	natsURL := fs.String("nats", "nats://auth-server:auth-secret@localhost:4222", "NATS URL")
	arn := fs.String("arn", "", "Resource ARN")
	fs.Parse(os.Args[2:])

	if *arn == "" {
		log.Fatal("arn is required")
	}

	nc, _ := nats.Connect(*natsURL)
	defer nc.Close()

	req := map[string]interface{}{
		"correlation_id": uuid.New().String(),
		"resource_arn":   *arn,
	}
	data, _ := json.Marshal(req)

	msg, err := nc.Request("dev.network.v1.resource.resolve", data, nats.DefaultTimeout)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	fmt.Printf("Resolution Response: %s\n", string(msg.Data))
}

func listResourcesCmd() {
	fs := flag.NewFlagSet("list-resources", flag.ExitOnError)
	natsURL := fs.String("nats", "nats://auth-server:auth-secret@localhost:4222", "NATS URL")
	tenantID := fs.String("tenant", "tenant-001", "Tenant ID")
	vpcID := fs.String("vpc", "", "VPC ID")
	fs.Parse(os.Args[2:])

	if *vpcID == "" {
		log.Fatal("vpc is required")
	}

	nc, _ := nats.Connect(*natsURL)
	defer nc.Close()

	req := map[string]interface{}{
		"correlation_id": uuid.New().String(),
		"tenant_id":      *tenantID,
		"vpc_id":         *vpcID,
	}
	data, _ := json.Marshal(req)

	msg, err := nc.Request("dev.network.v1.vpc.resources.list", data, nats.DefaultTimeout)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	fmt.Printf("VPC Resources: %s\n", string(msg.Data))
}

func resolveDefaultCmd() {
	fs := flag.NewFlagSet("resolve-default", flag.ExitOnError)
	natsURL := fs.String("nats", "nats://auth-server:auth-secret@localhost:4222", "NATS URL")
	tenantID := fs.String("tenant", "tenant-001", "Tenant ID")
	fs.Parse(os.Args[2:])

	nc, _ := nats.Connect(*natsURL)
	defer nc.Close()

	req := map[string]interface{}{
		"correlation_id": uuid.New().String(),
		"tenant_id":      *tenantID,
	}
	data, _ := json.Marshal(req)

	msg, err := nc.Request("dev.network.v1.vpc.default.resolve", data, nats.DefaultTimeout)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	fmt.Printf("Default Resolution: %s\n", string(msg.Data))
}

func registerComputeCmd() {
	fs := flag.NewFlagSet("register-compute", flag.ExitOnError)
	natsURL := fs.String("nats", "nats://auth-server:auth-secret@localhost:4222", "NATS URL")
	id := fs.String("id", "", "Instance ID")
	ip := fs.String("ip", "", "IP Address")
	port := fs.Int("port", 80, "Service Port")
	metadataStr := fs.String("metadata", "{}", "Metadata JSON (e.g. '{\"zone\":\"us-east-1\",\"type\":\"t2.micro\"}')")
	fs.Parse(os.Args[2:])

	if *id == "" || *ip == "" {
		log.Fatal("id and ip are required")
	}

	var metadata map[string]string
	if err := json.Unmarshal([]byte(*metadataStr), &metadata); err != nil {
		log.Fatalf("Invalid metadata JSON: %v", err)
	}

	nc, _ := nats.Connect(*natsURL)
	defer nc.Close()

	req := map[string]interface{}{
		"correlation_id": uuid.New().String(),
		"instance_id":    *id,
		"ip_address":     *ip,
		"service_port":   *port,
		"metadata":       metadata,
	}
	data, _ := json.Marshal(req)

	msg, err := nc.Request("dev.network.v1.compute.register", data, nats.DefaultTimeout)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	fmt.Printf("Registration Response: %s\n", string(msg.Data))
}

func computeHealthCmd() {
	fs := flag.NewFlagSet("compute-health", flag.ExitOnError)
	natsURL := fs.String("nats", "nats://auth-server:auth-secret@localhost:4222", "NATS URL")
	id := fs.String("id", "", "Instance ID")
	status := fs.String("status", "healthy", "Status (healthy, unhealthy, starting, stopping)")
	fs.Parse(os.Args[2:])

	if *id == "" {
		log.Fatal("id is required")
	}

	nc, _ := nats.Connect(*natsURL)
	defer nc.Close()

	req := map[string]interface{}{
		"correlation_id": uuid.New().String(),
		"instance_id":    *id,
		"status":         *status,
	}
	data, _ := json.Marshal(req)

	msg, err := nc.Request("dev.network.v1.compute.health", data, nats.DefaultTimeout)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	fmt.Printf("Health Update Response: %s\n", string(msg.Data))
}

func deregisterComputeCmd() {
	fs := flag.NewFlagSet("deregister-compute", flag.ExitOnError)
	natsURL := fs.String("nats", "nats://auth-server:auth-secret@localhost:4222", "NATS URL")
	id := fs.String("id", "", "Instance ID")
	fs.Parse(os.Args[2:])

	if *id == "" {
		log.Fatal("id is required")
	}

	nc, _ := nats.Connect(*natsURL)
	defer nc.Close()

	req := map[string]interface{}{
		"correlation_id": uuid.New().String(),
		"instance_id":    *id,
	}
	data, _ := json.Marshal(req)

	msg, err := nc.Request("dev.network.v1.compute.deregister", data, nats.DefaultTimeout)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	fmt.Printf("Deregistration Response: %s\n", string(msg.Data))
}

func listComputeCmd() {
	fs := flag.NewFlagSet("list-compute", flag.ExitOnError)
	natsURL := fs.String("nats", "nats://auth-server:auth-secret@localhost:4222", "NATS URL")
	fs.Parse(os.Args[2:])

	nc, _ := nats.Connect(*natsURL)
	defer nc.Close()

	req := map[string]interface{}{
		"correlation_id": uuid.New().String(),
	}
	data, _ := json.Marshal(req)

	msg, err := nc.Request("dev.network.v1.compute.list", data, nats.DefaultTimeout)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	fmt.Printf("Compute Instances: %s\n", string(msg.Data))
}

func computeLifecycleCmd() {
	fs := flag.NewFlagSet("compute-lifecycle", flag.ExitOnError)
	natsURL := fs.String("nats", "nats://auth-server:auth-secret@localhost:4222", "NATS URL")
	id := fs.String("id", "", "Instance ID")
	eventType := fs.String("type", "INSTANCE_STARTED", "Event Type (INSTANCE_STARTED, INSTANCE_STOPPED, HEALTH_UPDATE)")
	ip := fs.String("ip", "10.0.1.50", "IP Address")
	port := fs.Int("port", 80, "Service Port")
	metadataStr := fs.String("metadata", "{}", "Metadata JSON")
	fs.Parse(os.Args[2:])

	if *id == "" {
		log.Fatal("id is required")
	}

	var metadata map[string]string
	if err := json.Unmarshal([]byte(*metadataStr), &metadata); err != nil {
		log.Fatalf("Invalid metadata JSON: %v", err)
	}

	nc, _ := nats.Connect(*natsURL)
	defer nc.Close()

	payload := map[string]interface{}{
		"ip_address":   *ip,
		"service_port": *port,
		"metadata":     metadata,
	}

	event := map[string]interface{}{
		"correlation_id": uuid.New().String(),
		"instance_id":    *id,
		"event_type":     *eventType,
		"timestamp":      time.Now(),
		"payload":        payload,
	}
	data, _ := json.Marshal(event)

	err := nc.Publish("dev.compute.v1.instance.lifecycle", data)
	if err != nil {
		log.Fatalf("Publish failed: %v", err)
	}
	fmt.Printf("Published lifecycle event: %s\n", string(data))
}

func routeComputeCmd() {
	fs := flag.NewFlagSet("route-compute", flag.ExitOnError)
	natsURL := fs.String("nats", "nats://auth-server:auth-secret@localhost:4222", "NATS URL")
	fs.Parse(os.Args[2:])

	nc, _ := nats.Connect(*natsURL)
	defer nc.Close()

	req := map[string]interface{}{
		"correlation_id": uuid.New().String(),
	}
	data, _ := json.Marshal(req)

	msg, err := nc.Request("dev.network.v1.compute.route", data, nats.DefaultTimeout)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	fmt.Printf("Selected Route: %s\n", string(msg.Data))
}
