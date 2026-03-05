package domain

import "time"

type InstanceStatus string

const (
	InstanceStatusHealthy   InstanceStatus = "healthy"
	InstanceStatusUnhealthy InstanceStatus = "unhealthy"
	InstanceStatusStarting  InstanceStatus = "starting"
	InstanceStatusStopping  InstanceStatus = "stopping"
)

type ComputeEventType string

const (
	ComputeEventInstanceStarted ComputeEventType = "INSTANCE_STARTED"
	ComputeEventInstanceStopped ComputeEventType = "INSTANCE_STOPPED"
	ComputeEventHealthUpdate    ComputeEventType = "HEALTH_UPDATE"
)

type ComputeInstance struct {
	InstanceID       string            `json:"instance_id"`
	IPAddress        string            `json:"ip_address"`
	ServicePort      int               `json:"service_port"`
	Status           InstanceStatus    `json:"status"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	LastHealthUpdate time.Time         `json:"last_health_update"`
}
