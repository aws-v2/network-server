package registry

import (
	"fmt"
	"sync"
	"time"

	"github.com/martin/network-service/internal/domain"
)

type ComputeRegistry interface {
	Register(instance domain.ComputeInstance) error
	UpdateHealth(instanceID string, status domain.InstanceStatus) error
	Deregister(instanceID string) error
	GetActiveInstances() ([]domain.ComputeInstance, error)
	SelectRoute() (*domain.ComputeInstance, error)
	CleanupStaleInstances(threshold time.Duration) ([]string, error)
}

type inMemoryComputeRegistry struct {
	mu         sync.RWMutex
	instances  map[string]domain.ComputeInstance
	roundRobin int
}

func NewComputeRegistry() ComputeRegistry {
	return &inMemoryComputeRegistry{
		instances: make(map[string]domain.ComputeInstance),
	}
}

func (r *inMemoryComputeRegistry) Register(instance domain.ComputeInstance) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	instance.LastHealthUpdate = time.Now()
	r.instances[instance.InstanceID] = instance
	return nil
}

func (r *inMemoryComputeRegistry) UpdateHealth(instanceID string, status domain.InstanceStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	instance, ok := r.instances[instanceID]
	if !ok {
		return fmt.Errorf("instance not found: %s", instanceID)
	}

	instance.Status = status
	instance.LastHealthUpdate = time.Now()
	r.instances[instanceID] = instance
	return nil
}

func (r *inMemoryComputeRegistry) Deregister(instanceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.instances, instanceID)
	return nil
}

func (r *inMemoryComputeRegistry) GetActiveInstances() ([]domain.ComputeInstance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var active []domain.ComputeInstance
	for _, inst := range r.instances {
		if inst.Status == domain.InstanceStatusHealthy {
			active = append(active, inst)
		}
	}
	return active, nil
}

func (r *inMemoryComputeRegistry) SelectRoute() (*domain.ComputeInstance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var healthy []domain.ComputeInstance
	now := time.Now()
	threshold := 60 * time.Second

	for _, inst := range r.instances {
		// Only select healthy instances with a recent heartbeat
		if inst.Status == domain.InstanceStatusHealthy && now.Sub(inst.LastHealthUpdate) < threshold {
			healthy = append(healthy, inst)
		}
	}

	if len(healthy) == 0 {
		return nil, fmt.Errorf("no healthy compute instances available")
	}

	// Simple Round-Robin
	r.roundRobin = (r.roundRobin + 1) % len(healthy)
	selected := healthy[r.roundRobin]

	return &selected, nil
}

func (r *inMemoryComputeRegistry) CleanupStaleInstances(threshold time.Duration) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var evicted []string
	now := time.Now()

	for id, inst := range r.instances {
		if now.Sub(inst.LastHealthUpdate) > threshold {
			evicted = append(evicted, id)
			delete(r.instances, id)
		}
	}

	return evicted, nil
}
