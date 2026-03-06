package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// ExposeRDSContainer allocates a public port and sets up DNAT/SNAT rules
func (s *networkService) ExposeRDSContainer(ctx context.Context, tenantID, resourceID, privateIP, publicIP string, privatePort int) (int, error) {
	// 1. Allocate a port
	publicPort, err := s.rdsPortRepo.Allocate(ctx, tenantID, resourceID, privateIP, privatePort, publicIP)
	if err != nil {
		zap.L().Error("[RDS-EXPOSE] Failed to allocate port", zap.Error(err), zap.String("resource_id", resourceID))
		return 0, fmt.Errorf("failed to allocate RDS port: %w", err)
	}

	zap.L().Info("[RDS-EXPOSE] Allocated port", zap.Int("public_port", publicPort), zap.String("resource_id", resourceID))

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

	zap.L().Info("[RDS-EXPOSE] Successfully exposed RDS container",
		zap.String("resource_id", resourceID),
		zap.Int("public_port", publicPort),
	)

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
		zap.L().Info("[RDS-EXPOSE] No active port allocation found, nothing to unexpose", zap.String("resource_id", resourceID))
		return nil
	}

	zap.L().Info("[RDS-EXPOSE] Unexposing RDS container", zap.String("resource_id", resourceID), zap.Int("public_port", alloc.PublicPort))

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

	zap.L().Info("[RDS-EXPOSE] Successfully unexposed RDS container", zap.String("resource_id", resourceID))

	return nil
}

// ReconcileRDSPorts re-applies DNAT/SNAT rules for all active RDS port allocations.
// iptables rules are lost on host restart; this is called at startup alongside ReconcileVPCs.
func (s *networkService) ReconcileRDSPorts(ctx context.Context) error {
	allocs, err := s.rdsPortRepo.ListActive(ctx)
	if err != nil {
		zap.L().Error("[RDS-RECONCILE] Failed to list active port allocations", zap.Error(err))
		return fmt.Errorf("failed to list active RDS port allocations: %w", err)
	}

	if len(allocs) == 0 {
		zap.L().Info("[RDS-RECONCILE] No active RDS port allocations to reconcile")
		return nil
	}

	zap.L().Info("[RDS-RECONCILE] Reconciling RDS port rules", zap.Int("count", len(allocs)))

	var reconciledCount int
	for _, alloc := range allocs {
		log := zap.L().With(
			zap.String("resource_id", alloc.ResourceID),
			zap.String("public_ip", alloc.PublicIP),
			zap.Int("public_port", alloc.PublicPort),
			zap.String("private_ip", alloc.PrivateIP),
			zap.Int("private_port", alloc.PrivatePort),
		)

		if err := s.iptablesDriver.SetupRDSDNAT(alloc.PublicIP, alloc.PublicPort, alloc.PrivateIP, alloc.PrivatePort); err != nil {
			log.Error("[RDS-RECONCILE] Failed to re-apply DNAT rule", zap.Error(err))
			continue
		}

		if err := s.iptablesDriver.SetupRDSSNAT(alloc.PrivateIP, alloc.PrivatePort, alloc.PublicIP, alloc.PublicPort); err != nil {
			log.Error("[RDS-RECONCILE] Failed to re-apply SNAT rule", zap.Error(err))
			continue
		}

		log.Info("[RDS-RECONCILE] Successfully reconciled RDS port rules")
		reconciledCount++
	}

	zap.L().Info("[RDS-RECONCILE] Reconciliation complete",
		zap.Int("total", len(allocs)),
		zap.Int("reconciled", reconciledCount),
	)
	return nil
}
