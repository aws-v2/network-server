package service_test

import (
	"context"
	"testing"

	"github.com/martin/network-service/internal/domain"
	"github.com/martin/network-service/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRegisterComputeInstance(t *testing.T) {
	svc, mocks := service.NewTestService()
	ctx := context.Background()

	instance := domain.ComputeInstance{
		InstanceID: "inst-1",
		IPAddress:  "10.0.1.10",
	}

	mocks.ComputeReg.On("Register", instance).Return(nil)

	err := svc.RegisterComputeInstance(ctx, instance)

	assert.NoError(t, err)
	mocks.ComputeReg.AssertExpectations(t)
}

func TestHandleComputeLifecycleEvent_Started(t *testing.T) {
	svc, mocks := service.NewTestService()
	ctx := context.Background()

	event := domain.ComputeLifecycleEvent{
		InstanceID: "inst-1",
		EventType:  domain.ComputeEventInstanceStarted,
		Payload: domain.ComputeEventPayload{
			IPAddress:   "10.0.1.10",
			ServicePort: 8080,
		},
	}

	mocks.ComputeReg.On("Register", mock.Anything).Return(nil)

	err := svc.HandleComputeLifecycleEvent(ctx, event)

	assert.NoError(t, err)
	mocks.ComputeReg.AssertExpectations(t)
}
