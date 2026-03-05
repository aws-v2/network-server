package domain

import "time"

type UserRegisteredEvent struct {
	CorrelationID string `json:"correlation_id"`
	TenantID      string `json:"tenant_id"`
	TenantName    string `json:"tenant_name"`
	Email         string `json:"email"`
}

type CreateVPCEvent struct {
	CorrelationID string `json:"correlation_id"`
	TenantID      string `json:"tenant_id"`
	VPCName       string `json:"vpc_name"`
	RequestedBy   string `json:"requested_by"`
}

type VPCCreatedEvent struct {
	CorrelationID string `json:"correlation_id"`
	TenantID      string `json:"tenant_id"`
	VPCID         string `json:"vpc_id"`
	CIDRBlock     string `json:"cidr_block"`
	Status        string `json:"status"`
}

// Phase 3: Resolution & Validation
type GetDefaultVPCRequest struct {
	CorrelationID string `json:"correlation_id"`
	TenantID      string `json:"tenant_id"`
}

type GetDefaultVPCResponse struct {
	CorrelationID string `json:"correlation_id"`
	TenantID      string `json:"tenant_id"`
	VPCID         string `json:"vpc_id"`
	Status        string `json:"status"`
}

type ValidateVPCRequest struct {
	CorrelationID string `json:"correlation_id"`
	TenantID      string `json:"tenant_id"`
	VPCID         string `json:"vpc_id"`
}

type ValidateVPCResponse struct {
	CorrelationID string `json:"correlation_id"`
	Valid         bool   `json:"valid"`
	Status        string `json:"status"`
	Reason        string `json:"reason"`
}

// Phase 4 & 11: Resource Assignment (Subnet-level)
type AttachResourceRequest struct {
	CorrelationID  string `json:"correlation_id"`
	TenantID       string `json:"tenant_id"`
	ResourceARN    string `json:"resource_arn"`
	TargetVPCID    string `json:"target_vpc_id"`
	TargetSubnetID string `json:"target_subnet_id"`
}

type AttachResourceResponse struct {
	CorrelationID string `json:"correlation_id"`
	Success       bool   `json:"success"`
	VPCID         string `json:"vpc_id"`
	SubnetID      string `json:"subnet_id"`
	PrivateIP     string `json:"private_ip"`
	Message       string `json:"message"`
}

type DetachResourceRequest struct {
	CorrelationID string `json:"correlation_id"`
	TenantID      string `json:"tenant_id"`
	ResourceARN   string `json:"resource_arn"`
}

type DetachResourceResponse struct {
	CorrelationID string `json:"correlation_id"`
	Success       bool   `json:"success"`
	Message       string `json:"message"`
}

type ResolveResourceRequest struct {
	CorrelationID string `json:"correlation_id"`
	ResourceARN   string `json:"resource_arn"`
}

type ResolveResourceResponse struct {
	CorrelationID string `json:"correlation_id"`
	VPCID         string `json:"vpc_id"`
	SubnetID      string `json:"subnet_id"`
	PrivateIP     string `json:"private_ip"`
}

type ListVPCResourcesRequest struct {
	CorrelationID string `json:"correlation_id"`
	TenantID      string `json:"tenant_id"`
	VPCID         string `json:"vpc_id"`
}

type VPCResourceItem struct {
	ResourceARN string `json:"resource_arn"`
	SubnetID    string `json:"subnet_id"`
	PrivateIP   string `json:"private_ip"`
}

type ListVPCResourcesResponse struct {
	CorrelationID string            `json:"correlation_id"`
	Resources     []VPCResourceItem `json:"resources"`
}

// Phase 11.4: Default Resolution
type ResolveDefaultVPCRequest struct {
	CorrelationID string `json:"correlation_id"`
	TenantID      string `json:"tenant_id"`
}

type ResolveDefaultVPCResponse struct {
	CorrelationID string `json:"correlation_id"`
	VPCID         string `json:"vpc_id"`
	SubnetID      string `json:"subnet_id"`
	BridgeName    string `json:"bridge_name"`
}

// Phase 12: Compute Registry
type RegisterComputeRequest struct {
	CorrelationID string            `json:"correlation_id"`
	InstanceID    string            `json:"instance_id"`
	IPAddress     string            `json:"ip_address"`
	ServicePort   int               `json:"service_port"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

type UpdateComputeHealthRequest struct {
	CorrelationID string         `json:"correlation_id"`
	InstanceID    string         `json:"instance_id"`
	Status        InstanceStatus `json:"status"`
}

type DeregisterComputeRequest struct {
	CorrelationID string `json:"correlation_id"`
	InstanceID    string `json:"instance_id"`
}

type ListComputeInstancesRequest struct {
	CorrelationID string `json:"correlation_id"`
}

type ListComputeInstancesResponse struct {
	CorrelationID string            `json:"correlation_id"`
	Instances     []ComputeInstance `json:"instances"`
}

type SelectComputeRouteRequest struct {
	CorrelationID string `json:"correlation_id"`
}

type SelectComputeRouteResponse struct {
	CorrelationID string           `json:"correlation_id"`
	Success       bool             `json:"success"`
	Instance      *ComputeInstance `json:"instance,omitempty"`
	Error         string           `json:"error,omitempty"`
}

type ComputeEventPayload struct {
	IPAddress   string            `json:"ip_address,omitempty"`
	ServicePort int               `json:"service_port,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type ComputeLifecycleEvent struct {
	CorrelationID string              `json:"correlation_id"`
	InstanceID    string              `json:"instance_id"`
	EventType     ComputeEventType    `json:"event_type"`
	Timestamp     time.Time           `json:"timestamp"`
	Payload       ComputeEventPayload `json:"payload"`
}
