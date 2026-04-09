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

	allVPCs, err := s.vpcRepo.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to list VPCs for reconciliation: %w", err)
	}

	if len(allVPCs) == 0 {
		l.Info("No VPCs found, nothing to reconcile")
		return nil
	}

	for _, vpc := range allVPCs {
		vpcLog := l.With(
			zap.String("vpc_id", vpc.ID),
			zap.String("bridge", vpc.BridgeName),
			zap.String("status", vpc.Status),
		)

		// ── Step 1: Ensure bridge exists ─────────────────────────────────────
		exists, err := s.bridgeDriver.Exists(vpc.BridgeName)
		if err != nil {
			vpcLog.Error("Reconciliation: failed to check bridge existence", zap.Error(err))
			continue
		}

		if !exists {
			vpcLog.Info("Bridge missing from OS, recreating")
			if err := s.bridgeDriver.CreateBridge(vpc.BridgeName); err != nil {
				vpcLog.Error("Reconciliation: bridge creation error", zap.Error(err))
				s.vpcRepo.UpdateStatus(ctx, vpc.ID, domain.VPCStatusError)
				continue
			}
			vpcLog.Info("Bridge recreated successfully")
		}

		// ── Step 2: Always bring bridge UP ───────────────────────────────────
		// This is the critical fix.
		// CreateBridge brings the bridge up on first creation, but if the bridge
		// already existed (exists=true above), BringUp was never called after a
		// reboot. Bridges survive reboots as kernel objects but come back DOWN
		// with no IPs. We always call BringUp regardless of whether we just
		// created it or it already existed.
		if err := s.bridgeDriver.BringUp(vpc.BridgeName); err != nil {
			vpcLog.Error("Reconciliation: failed to bring bridge up", zap.Error(err))
			// Non-fatal — log and continue, AssignIP may still work
		} else {
			vpcLog.Info("Bridge is UP")
		}

		// We don't register the bridge with Docker here yet because we need the
		// public subnet CIDR and gateway ID, which we'll find below.
		var publicSubnetCIDR string
		var publicGatewayIP string

		// ── Step 3: Assign gateway IP for every subnet in this VPC ───────────
		// Each subnet (e.g. 10.1.1.0/24) needs its gateway (10.1.1.1/24)
		// assigned to the bridge so VMs can reach their default gateway.
		// We do ALL subnets, not just public, because private subnet VMs also
		// need a gateway on the bridge to communicate.
		subnets, err := s.subnetRepo.ListByVPC(ctx, vpc.ID)
		if err != nil {
			vpcLog.Error("Reconciliation: could not list subnets", zap.Error(err))
		} else {
			for _, sn := range subnets {
				gateway := DeriveGateway(sn.CIDRBlock)
				if gateway == "" {
					vpcLog.Warn("Could not derive gateway for subnet", zap.String("subnet_cidr", sn.CIDRBlock))
					continue
				}

				gatewayCIDR := fmt.Sprintf("%s/24", gateway)
				if err := s.bridgeDriver.AssignIP(vpc.BridgeName, gatewayCIDR); err != nil {
					vpcLog.Error("Reconciliation: gateway IP assignment error",
						zap.Error(err),
						zap.String("subnet", sn.Name),
						zap.String("gateway_cidr", gatewayCIDR),
					)
				} else {
					vpcLog.Info("Gateway IP assigned to bridge",
						zap.String("subnet", sn.Name),
						zap.String("gateway_cidr", gatewayCIDR),
					)
				}

				// ── Step 4: Re-apply MASQUERADE for public subnets ────────────
				// This allows VMs to reach the internet through the host.
				// Private subnets intentionally do not get MASQUERADE.
				if strings.Contains(strings.ToLower(sn.Name), "public") {
					publicSubnetCIDR = sn.CIDRBlock
					publicGatewayIP = gateway

					if err := s.iptablesDriver.SetupMasquerade(sn.CIDRBlock, vpc.BridgeName); err != nil {
						vpcLog.Error("Reconciliation: MASQUERADE setup error",
							zap.Error(err),
							zap.String("subnet_cidr", sn.CIDRBlock),
						)
					} else {
						vpcLog.Info("MASQUERADE rule applied",
							zap.String("subnet_cidr", sn.CIDRBlock),
						)
					}
				}
			}

			// ── Step 4.5: Re-register Docker Bridge ───────────────────────────
			if publicSubnetCIDR != "" && publicGatewayIP != "" {
				if err := s.dockerNetworkDriver.RegisterBridge(vpc.BridgeName, publicSubnetCIDR, publicGatewayIP); err != nil {
					vpcLog.Warn("Reconciliation: failed to register bridge with Docker", zap.Error(err))
					// Non-fatal — log and continue
				} else {
					vpcLog.Info("Bridge re-registered with Docker")
				}
			} else {
				vpcLog.Error("Reconciliation: failed to derive public subnet or gateway for Docker network registration")
			}
		}

		// ── Step 5: Mark VPC active ───────────────────────────────────────────
		if err := s.vpcRepo.UpdateStatus(ctx, vpc.ID, domain.VPCStatusActive); err != nil {
			vpcLog.Error("Reconciliation: could not update VPC status to active", zap.Error(err))
			continue
		}

		vpcLog.Info("VPC reconciled and marked active")
	}

	return nil
}

// DeriveGateway returns the gateway IP for a given subnet CIDR.
// Convention: gateway is always the first usable IP (x.x.x.1).
// Examples:
//
//	10.1.1.0/24  → 10.1.1.1
//	10.1.2.0/24  → 10.1.2.1
func DeriveGateway(cidr string) string {
	// cidr is e.g. "10.1.1.0/24"
	// strip the prefix length and replace last octet with 1
	parts := strings.Split(cidr, "/")
	if len(parts) == 0 {
		return ""
	}
	ip := parts[0] // "10.1.1.0"
	octets := strings.Split(ip, ".")
	if len(octets) != 4 {
		return ""
	}
	// Replace last octet with 1 → "10.1.1.1"
	return fmt.Sprintf("%s.%s.%s.1", octets[0], octets[1], octets[2])
}
