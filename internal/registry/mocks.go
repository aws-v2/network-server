package registry

import (
	"time"

	"github.com/martin/network-service/internal/domain"
	"github.com/stretchr/testify/mock"
)

type MockComputeRegistry struct {
	mock.Mock
}

func (m *MockComputeRegistry) Register(instance domain.ComputeInstance) error {
	args := m.Called(instance)
	return args.Error(0)
}

func (m *MockComputeRegistry) UpdateHealth(instanceID string, status domain.InstanceStatus) error {
	args := m.Called(instanceID, status)
	return args.Error(0)
}

func (m *MockComputeRegistry) Deregister(instanceID string) error {
	args := m.Called(instanceID)
	return args.Error(0)
}

func (m *MockComputeRegistry) GetActiveInstances() ([]domain.ComputeInstance, error) {
	args := m.Called()
	return args.Get(0).([]domain.ComputeInstance), args.Error(1)
}

func (m *MockComputeRegistry) SelectRoute() (*domain.ComputeInstance, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ComputeInstance), args.Error(1)
}

func (m *MockComputeRegistry) CleanupStaleInstances(threshold time.Duration) ([]string, error) {
	args := m.Called(threshold)
	return args.Get(0).([]string), args.Error(1)
}
