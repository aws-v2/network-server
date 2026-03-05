package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/martin/network-service/internal/domain"
	"github.com/martin/network-service/pkg/logger"
	"go.uber.org/zap"
)

func (s *networkService) ReconcileVPCs(ctx context.Context) error {
	l := logger.WithContext(ctx)
	l.Info("Starting VPC reconciliation loop")

	// List ALL VPCs, not just incomplete ones.
	// Active VPCs lose their OS bridges after a host reboot since Linux bridges are not persistent.
	allVPCs, err := s.vpcRepo.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to list VPCs for reconciliation: %w", err)
	}

	if len(allVPCs) == 0 {
		l.Info("No VPCs found, nothing to reconcile")
		return nil
	}

	for _, vpc := range allVPCs {
		vpcLog := l.With(zap.String("vpc_id", vpc.ID), zap.String("bridge", vpc.BridgeName), zap.String("status", vpc.Status))

		// 1. Check if the OS bridge actually exists
		exists, err := s.bridgeDriver.Exists(vpc.BridgeName)
		if err != nil {
			vpcLog.Error("Reconciliation: failed to check bridge existence", zap.Error(err))
			continue
		}

		if exists {
			// Bridge is already up on the OS — nothing to do for this VPC
			if vpc.Status != domain.VPCStatusActive {
				// DB is stale; update it
				if err := s.vpcRepo.UpdateStatus(ctx, vpc.ID, domain.VPCStatusActive); err != nil {
					vpcLog.Error("Reconciliation: could not update stale VPC status", zap.Error(err))
				}
			}
			vpcLog.Debug("Bridge already exists on OS, skipping")
			continue
		}

		// Bridge is missing from the OS — recreate it
		vpcLog.Info("Bridge missing from OS, recreating")

		if err := s.bridgeDriver.CreateBridge(vpc.BridgeName); err != nil {
			vpcLog.Error("Reconciliation failed: bridge creation error", zap.Error(err))
			s.vpcRepo.UpdateStatus(ctx, vpc.ID, domain.VPCStatusError)
			continue
		}
		vpcLog.Info("Bridge recreated successfully")

		// Assign gateway IP from the VPC CIDR
		octets := strings.Split(vpc.CIDRBlock, ".")
		if len(octets) >= 2 {
			basePrefix := fmt.Sprintf("%s.%s", octets[0], octets[1])
			gatewayCIDR := fmt.Sprintf("%s.1.1/24", basePrefix)
			if err := s.bridgeDriver.AssignIP(vpc.BridgeName, gatewayCIDR); err != nil {
				vpcLog.Error("Reconciliation: bridge IP assignment error", zap.Error(err))
				// Non-fatal: bridge is up, IP might already be there or can be set later
			} else {
				vpcLog.Info("Gateway IP assigned to bridge", zap.String("cidr", gatewayCIDR))
			}
		}

		// Re-apply MASQUERADE rules for public subnets
		subnets, err := s.subnetRepo.ListByVPC(ctx, vpc.ID)
		if err != nil {
			vpcLog.Error("Reconciliation: could not list subnets for iptables", zap.Error(err))
		} else {
			for _, sn := range subnets {
				if strings.Contains(sn.Name, "public") {
					if err := s.iptablesDriver.SetupMasquerade(sn.CIDRBlock); err != nil {
						vpcLog.Error("Reconciliation: MASQUERADE setup error", zap.Error(err), zap.String("subnet_id", sn.ID))
					}
				}
			}
		}

		// Mark VPC active
		if err := s.vpcRepo.UpdateStatus(ctx, vpc.ID, domain.VPCStatusActive); err != nil {
			vpcLog.Error("Reconciliation: could not update VPC status to active", zap.Error(err))
			continue
		}

		vpcLog.Info("VPC bridge reconciled and marked active")
	}

	return nil
}
