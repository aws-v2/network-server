package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/martin/network-service/internal/domain"
	"github.com/martin/network-service/pkg/logger"
	"go.uber.org/zap"
)

func (s *networkService) CreateRouteTable(ctx context.Context, vpcID, name string) (*domain.RouteTable, error) {
	rtID := uuid.New().String()
	rt := &domain.RouteTable{
		ID:        rtID,
		VPCID:     vpcID,
		Name:      name,
		CreatedAt: time.Now(),
	}

	if err := s.rtRepo.Create(ctx, rt); err != nil {
		return nil, err
	}
	return rt, nil
}

func (s *networkService) AssociateSubnetToRouteTable(ctx context.Context, subnetID, rtID string) error {
	l := logger.WithContext(ctx)

	// 1. Update database
	if err := s.subnetRepo.AssociateRouteTable(ctx, subnetID, rtID); err != nil {
		return err
	}

	// 2. Fetch details for OS orchestration
	subnet, err := s.subnetRepo.GetByID(ctx, subnetID)
	if err != nil {
		return fmt.Errorf("failed to fetch subnet: %w", err)
	}

	vpc, err := s.vpcRepo.GetByID(ctx, subnet.VPCID)
	if err != nil {
		return fmt.Errorf("failed to fetch VPC: %w", err)
	}

	rt, err := s.rtRepo.GetFullByID(ctx, rtID)
	if err != nil {
		return fmt.Errorf("failed to fetch route table details: %w", err)
	}

	// 3. Apply existing routes to the bridge
	for _, route := range rt.Routes {
		// For now, we only apply routes that have a target ID (which we treat as the gateway for now)
		// and skip those that don't make sense for a direct 'ip route add ... via ... dev ...'
		if route.TargetID != "" {
			if err := s.routingDriver.AddRoute(vpc.BridgeName, route.DestinationCIDR, route.TargetID); err != nil {
				l.Error("Failed to apply route during association",
					zap.Error(err),
					zap.String("bridge", vpc.BridgeName),
					zap.String("destination", route.DestinationCIDR))
			}
		}
	}

	l.Info("Associated subnet to route table and applied routes",
		zap.String("subnet_id", subnetID),
		zap.String("rt_id", rtID),
		zap.String("bridge", vpc.BridgeName))
	return nil
}

func (s *networkService) CreateRoute(ctx context.Context, rtID, destination, targetType, targetID string) error {
	l := logger.WithContext(ctx)

	// 1. Save in database
	route := &domain.Route{
		ID:              uuid.New().String(),
		RouteTableID:    rtID,
		DestinationCIDR: destination,
		TargetType:      targetType,
		TargetID:        targetID,
		CreatedAt:       time.Now(),
	}

	if err := s.routeRepo.Create(ctx, route); err != nil {
		return err
	}

	// 2. Orhcestrate at OS level for affected subnets
	subnets, err := s.subnetRepo.ListByRouteTable(ctx, rtID)
	if err != nil {
		l.Error("Failed to list associated subnets for routing", zap.Error(err), zap.String("rt_id", rtID))
		return nil // Non-blocking for DB success
	}

	for _, sn := range subnets {
		vpc, err := s.vpcRepo.GetByID(ctx, sn.VPCID)
		if err != nil {
			l.Error("Failed to fetch VPC for routing orchestration", zap.Error(err), zap.String("vpc_id", sn.VPCID))
			continue
		}

		// Apply route to the bridge
		// Treating TargetID as gateway for IP routes
		if targetID != "" {
			if err := s.routingDriver.AddRoute(vpc.BridgeName, destination, targetID); err != nil {
				l.Error("Failed to apply route to system",
					zap.Error(err),
					zap.String("bridge", vpc.BridgeName),
					zap.String("destination", destination))
			}
		}
	}

	l.Info("Successfully created route and applied to OS",
		zap.String("rt_id", rtID),
		zap.String("destination", destination),
		zap.String("target", targetID),
		zap.Int("affected_subnets", len(subnets)))
	return nil
}
