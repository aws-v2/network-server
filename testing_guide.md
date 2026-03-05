# Network Service Testing Guide

This guide details how to perform the advanced tests described in the project walkthrough.

## 1. Setup
Ensure the Network Service is running:
```powershell
go run cmd/api/main.go
```

## 2. Using the Tester Utility
The `cmd/tester` tool handles NATS events and HTTP calls without quoting issues.

### Register a Tenant
```powershell
go run cmd/tester/main.go register --tenant tenant-001 --name "Test Tenant"
```

### Batch Concurrent Registration (Phase 2 & 8)
```powershell
go run cmd/tester/main.go batch --count 10 --concurrent true
```

### EIP Operations (Phase 6)
```powershell
go run cmd/tester/main.go eip-assoc --eip eip-123 --instance inst-456 --ip 10.1.1.50
### Custom VPC Provisioning (Phase 10)
```powershell
# Publish create event using mock_pub or tester (if updated)
# This simulates the EC2 service request
# Payload: {"tenant_id": "tenant-001", "vpc_name": "custom-vpc-1"}
go run cmd/tester/main.go publish-vpc-create --tenant tenant-001 --name "Custom VPC"

### VPC Resolution & Validation (Phase 3)
```powershell
# Resolve default VPC
go run cmd/tester/main.go resolve-default --tenant tenant-001
# Validate VPC ownership
go run cmd/tester/main.go validate-vpc --tenant tenant-001 --vpc <vpc_id>
```

### Resource Assignment (Phase 4)
```powershell
# Attach EC2/RDS to VPC
go run cmd/tester/main.go attach-resource --tenant tenant-001 --arn "arn:aws:ec2:inst-123" --vpc <vpc_id>
# Detach resource
go run cmd/tester/main.go detach-resource --arn "arn:aws:ec2:inst-123"

### Subnet-Level Networking (Phase 11)
```powershell
# Attach resource to specific Subnet
go run cmd/tester/main.go attach-subnet --tenant tenant-001 --arn "arn:aws:ec2:inst-1" --vpc <vpc_id> --subnet <subnet_id>

# Resolve resource placement
go run cmd/tester/main.go resolve-resource --arn "arn:aws:ec2:inst-1"

# List resources in VPC
go run cmd/tester/main.go list-resources --tenant tenant-001 --vpc <vpc_id>

# Resolve default network placement
go run cmd/tester/main.go resolve-default-network --tenant tenant-001
```

### NATS CLI Verification (Phase 7)
```powershell
# Attach Resource
nats req dev.network.v1.resource.attach '{
 "correlation_id":"test-attach-1",
 "tenant_id":"tenant-001",
 "resource_arn":"arn:cloud:ec2:inst-123",
 "vpc_id":"vpc-1",
 "subnet_id":"subnet-1"
}'

# Resolve Resource
nats req dev.network.v1.resource.resolve '{
 "correlation_id":"test-resolve-1",
 "resource_arn":"arn:cloud:ec2:inst-123"
}'

# List VPC Resources
nats req dev.network.v1.vpc.resources.list '{
 "correlation_id":"test-list-1",
 "tenant_id":"tenant-001",
 "vpc_id":"vpc-1"
}'

# Resolve Default Network
nats req dev.network.v1.vpc.default.resolve '{
 "correlation_id":"test-default-1",
 "tenant_id":"tenant-001"
}'

# Simulate Compute Lifecycle Events (Phase 13)
nats pub dev.compute.v1.instance.lifecycle '{
 "correlation_id":"lifecycle-1",
 "instance_id":"inst-789",
 "event_type":"INSTANCE_STARTED",
 "payload": { "ip_address": "10.0.1.100", "service_port": 80 }
}'

go run cmd/tester/main.go compute-lifecycle --id "inst-789" --type "INSTANCE_STOPPED"

# Verify Service Routing (Phase 14)
nats req dev.network.v1.compute.route '{
 "correlation_id":"route-test-1"
}'

go run cmd/tester/main.go route-compute
```
```
```

## 3. Failure Injection (Phase 1 & 5)
To test **Atomicity** and **Convergence**, you can inject errors or panics in `internal/service/service.go`.

### Force Transaction Failure
Navigate to `CreateDefaultVPC` in `service.go`.
Add this after `s.vpcRepo.CreateWithTx(...)`:
```go
return fmt.Errorf("forced failure for atomicity test")
```
Run `register` and verify NO row exists in the `vpcs` table.

### Simulate Crash
Add this after `s.bridgeDriver.CreateBridge(...)`:
```go
panic("crash after bridge created")
```
Run `register`. The bridge will exist, but the VPC record will be stuck in `pending` or not exist. Restart the service to see the **Reconciliation Loop** in action.

## 4. Environment Verification
> [!IMPORTANT]
> Bridge and Iptables tests (Phases 3-5) require a **Linux** environment.
> - If on Windows, use **WSL2** for actual networking.
> - If running on Windows natively, drivers will log "Skipping: not on Linux".

### Check Bridge
```bash
ip link show
```

### Check Iptables
```bash
sudo iptables -t nat -L -n -v
```
