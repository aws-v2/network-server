package service

import (
	"context"
	"fmt"

	"github.com/martin/network-service/pkg/logger"
	"go.uber.org/zap"
)

// ExposeRDSContainer allocates a public port and sets up DNAT/SNAT rules
func (s *networkService) ExposeRDSContainer(ctx context.Context, tenantID, resourceID, privateIP, publicIP string, privatePort int) (int, error) {
	zap.L().Info(fmt.Sprintf("[RDS-EXPOSE] Exposing %s %s:%d via %s", resourceID, privateIP, privatePort, publicIP))

	// 1. Allocate a port
	publicPort, err := s.rdsPortRepo.Allocate(ctx, tenantID, resourceID, privateIP, privatePort, publicIP)
	if err != nil {
		zap.L().Error("[RDS-EXPOSE] Failed to allocate port", zap.Error(err), zap.String("resource_id", resourceID))
		return 0, fmt.Errorf("failed to allocate RDS port: %w", err)
	}

	// 2. Setup DNAT
	if err := s.iptablesDriver.SetupRDSDNAT(publicIP, publicPort, privateIP, privatePort); err != nil {
		zap.L().Error("[RDS-EXPOSE] Failed to setup DNAT, releasing port", zap.Error(err))
		s.rdsPortRepo.Release(ctx, resourceID)
		return 0, fmt.Errorf("failed to setup RDS DNAT: %w", err)
	}

	// 3. Setup SNAT
	if err := s.iptablesDriver.SetupRDSSNAT(privateIP, privatePort, publicIP, publicPort); err != nil {
		zap.L().Error("[RDS-EXPOSE] Failed to setup SNAT, rolling back DNAT and releasing port", zap.Error(err))
		s.iptablesDriver.RemoveRDSDNAT(publicIP, publicPort, privateIP, privatePort)
		s.rdsPortRepo.Release(ctx, resourceID)
		return 0, fmt.Errorf("failed to setup RDS SNAT: %w", err)
	}

	zap.L().Info(fmt.Sprintf("[RDS-EXPOSE] Successfully exposed %s at %s:%d", resourceID, publicIP, publicPort))

	return publicPort, nil
}

// UnexposeRDSContainer removes DNAT/SNAT rules and releases the allocated port
func (s *networkService) UnexposeRDSContainer(ctx context.Context, resourceID string) error {
	// 1. Get allocation details
	alloc, err := s.rdsPortRepo.GetByResourceID(ctx, resourceID)
	if err != nil {
		zap.L().Error("[RDS-EXPOSE] Failed to get port allocation details", zap.Error(err), zap.String("resource_id", resourceID))
		return fmt.Errorf("failed to get RDS port allocation: %w", err)
	}

	if alloc == nil {
		zap.L().Warn("[RDS-EXPOSE] No active port allocation found, nothing to unexpose", zap.String("resource_id", resourceID))
		return nil
	}

	// 2. Remove NAT rules
	if err := s.iptablesDriver.RemoveRDSDNAT(alloc.PublicIP, alloc.PublicPort, alloc.PrivateIP, alloc.PrivatePort); err != nil {
		zap.L().Warn("[RDS-EXPOSE] Error removing DNAT rule", zap.Error(err))
	}

	if err := s.iptablesDriver.RemoveRDSSNAT(alloc.PrivateIP, alloc.PrivatePort, alloc.PublicIP, alloc.PublicPort); err != nil {
		zap.L().Warn("[RDS-EXPOSE] Error removing SNAT rule", zap.Error(err))
	}

	// 3. Release port
	if err := s.rdsPortRepo.Release(ctx, resourceID); err != nil {
		zap.L().Error("[RDS-EXPOSE] Failed to release port", zap.Error(err))
		return fmt.Errorf("failed to release RDS port: %w", err)
	}

	zap.L().Info(fmt.Sprintf("[RDS-EXPOSE] Successfully unexposed %s", resourceID))

	return nil
}

// ReconcileRDSPorts re-applies DNAT/SNAT rules for all active RDS port allocations.
// iptables rules are lost on host restart; this is called at startup alongside ReconcileVPCs.
func (s *networkService) ReconcileRDSPorts(ctx context.Context) error {
	l := logger.WithContext(ctx)
	l.Info("[RDS-RECONCILE] Starting RDS port rules reconciliation")

	allocations, err := s.rdsPortRepo.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("failed to list active RDS port allocations: %w", err)
	}

	if len(allocations) == 0 {
		l.Info("[RDS-RECONCILE] No active RDS port allocations found")
		return nil
	}

	successCount := 0
	for _, alloc := range allocations {
		// Re-apply DNAT
		if err := s.iptablesDriver.SetupRDSDNAT(
			alloc.PublicIP, alloc.PublicPort,
			alloc.PrivateIP, alloc.PrivatePort,
		); err != nil {
			l.Error("[RDS-RECONCILE] Failed to re-apply DNAT rule",
				zap.String("resource_id", alloc.ResourceID),
				zap.Error(err))
			continue
		}

		// Re-apply SNAT
		if err := s.iptablesDriver.SetupRDSSNAT(
			alloc.PrivateIP, alloc.PrivatePort,
			alloc.PublicIP, alloc.PublicPort,
		); err != nil {
			l.Error("[RDS-RECONCILE] Failed to re-apply SNAT rule",
				zap.String("resource_id", alloc.ResourceID),
				zap.Error(err))
			continue
		}

		successCount++
		l.Info("[RDS-RECONCILE] Re-applied rules",
			zap.String("resource_id", alloc.ResourceID),
			zap.String("public", fmt.Sprintf("%s:%d", alloc.PublicIP, alloc.PublicPort)),
			zap.String("private", fmt.Sprintf("%s:%d", alloc.PrivateIP, alloc.PrivatePort)),
		)
	}

	l.Info("[RDS-RECONCILE] Reconciliation complete",
		zap.Int("total", len(allocations)),
		zap.Int("success", successCount),
	)
	return nil
}
