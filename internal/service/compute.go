package service

import (
	"context"
	"strings"
	"time"

	"github.com/martin/network-service/internal/domain"
	"github.com/martin/network-service/pkg/logger"
	"go.uber.org/zap"
)

func (s *networkService) RegisterComputeInstance(ctx context.Context, instance domain.ComputeInstance) error {
	l := logger.WithContext(ctx).With(zap.String("instance_id", instance.InstanceID))
	l.Info("Registering compute instance", zap.String("ip", instance.IPAddress))
	return s.computeReg.Register(instance)
}

func (s *networkService) UpdateComputeInstanceHealth(ctx context.Context, instanceID string, status domain.InstanceStatus) error {
	l := logger.WithContext(ctx).With(zap.String("instance_id", instanceID))
	l.Info("Updating compute instance health", zap.String("status", string(status)))
	return s.computeReg.UpdateHealth(instanceID, status)
}

func (s *networkService) DeregisterComputeInstance(ctx context.Context, instanceID string) error {
	l := logger.WithContext(ctx).With(zap.String("instance_id", instanceID))
	l.Info("Deregistering compute instance")
	return s.computeReg.Deregister(instanceID)
}

func (s *networkService) GetActiveComputeInstances(ctx context.Context) ([]domain.ComputeInstance, error) {
	l := logger.WithContext(ctx)
	l.Debug("Fetching active compute instances")
	return s.computeReg.GetActiveInstances()
}

func (s *networkService) HandleComputeLifecycleEvent(ctx context.Context, event domain.ComputeLifecycleEvent) error {
	l := logger.WithContext(ctx).With(
		zap.String("instance_id", event.InstanceID),
		zap.String("event_type", string(event.EventType)),
	)
	l.Info("Processing compute lifecycle event")

	switch event.EventType {
	case domain.ComputeEventInstanceStarted:
		if event.Payload.IPAddress == "" || event.Payload.ServicePort == 0 {
			l.Error("Cannot register instance: missing IP address or service port in INSTANCE_STARTED payload")
			return domain.ErrInvalidPayload
		}

		inst := domain.ComputeInstance{
			InstanceID:  event.InstanceID,
			IPAddress:   event.Payload.IPAddress,
			ServicePort: event.Payload.ServicePort,
			Status:      domain.InstanceStatusStarting,
			Metadata:    event.Payload.Metadata,
		}

		if err := s.computeReg.Register(inst); err != nil {
			l.Error("Failed to register compute instance on INSTANCE_STARTED", zap.Error(err))
			return err
		}

		l.Info("Compute instance registered successfully",
			zap.String("ip", inst.IPAddress),
			zap.Int("port", inst.ServicePort),
		)
		return nil

	case domain.ComputeEventInstanceStopped:
		if err := s.computeReg.Deregister(event.InstanceID); err != nil {
			// If already not found, treat as a no-op — it may have been cleaned up already
			if strings.Contains(err.Error(), "instance not found") {
				l.Warn("Instance already deregistered or never registered, skipping",
					zap.Error(err),
				)
				return nil
			}
			l.Error("Failed to deregister compute instance on INSTANCE_STOPPED", zap.Error(err))
			return err
		}

		l.Info("Compute instance deregistered successfully")
		return nil

	case domain.ComputeEventHealthUpdate:
		err := s.computeReg.UpdateHealth(event.InstanceID, domain.InstanceStatusHealthy)
		if err == nil {
			// Health updated successfully, nothing more to do
			return nil
		}

		if !strings.Contains(err.Error(), "instance not found") {
			// Unexpected error — propagate it
			l.Error("Failed to update compute instance health", zap.Error(err))
			return err
		}

		// Instance not found in registry — only auto-register if payload is valid.
		// A shut-off or unknown VM will have an empty IP/port, so we guard here
		// to avoid ghost entries for instances that were never properly started.
		if event.Payload.IPAddress == "" || event.Payload.ServicePort == 0 {
			l.Warn("Skipping auto-registration on health update: instance not found and payload is missing IP/port — VM may be shut off or event is stale",
				zap.String("instance_id", event.InstanceID),
				zap.Error(err),
			)
			return nil
		}

		l.Info("Instance not found during health update — auto-registering with valid payload",
			zap.String("ip", event.Payload.IPAddress),
			zap.Int("port", event.Payload.ServicePort),
			zap.Error(err),
		)

		inst := domain.ComputeInstance{
			InstanceID:  event.InstanceID,
			IPAddress:   event.Payload.IPAddress,
			ServicePort: event.Payload.ServicePort,
			Status:      domain.InstanceStatusHealthy,
			Metadata:    event.Payload.Metadata,
		}

		if regErr := s.computeReg.Register(inst); regErr != nil {
			l.Error("Failed to auto-register instance during health update fallback", zap.Error(regErr))
			return regErr
		}

		l.Info("Instance auto-registered successfully during health update fallback")
		return nil

	default:
		l.Warn("Unsupported compute lifecycle event type — ignoring",
			zap.String("event_type", string(event.EventType)),
		)
		return nil
	}
}

func (s *networkService) SelectComputeRoute(ctx context.Context) (*domain.ComputeInstance, error) {
	l := logger.WithContext(ctx)
	l.Info("Selecting compute route")

	instance, err := s.computeReg.SelectRoute()
	if err != nil {
		l.Error("Failed to select compute route", zap.Error(err))
		return nil, err
	}

	l.Info("Compute route selected",
		zap.String("instance_id", instance.InstanceID),
		zap.String("ip", instance.IPAddress),
	)
	return instance, nil
}

func (s *networkService) MonitorComputeHealth(ctx context.Context) error {
	zap.L().Info("Starting compute health monitor")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	threshold := 60 * time.Second

	for {
		select {
		case <-ctx.Done():
			zap.L().Info("Stopping compute health monitor — context cancelled")
			return ctx.Err()

		case <-ticker.C:
			evicted, err := s.computeReg.CleanupStaleInstances(threshold)
			if err != nil {
				zap.L().Error("Compute health cleanup failed", zap.Error(err))
				continue
			}

			if len(evicted) > 0 {
				zap.L().Warn("Evicted stale compute instances",
					zap.Strings("instance_ids", evicted),
					zap.Duration("threshold", threshold),
					zap.Int("count", len(evicted)),
				)
			} else {
				zap.L().Debug("Compute health cleanup completed — no stale instances found")
			}
		}
	}
}