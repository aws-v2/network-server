package service

import (
	"context"
	"database/sql"
	"fmt"
	"net"

	"github.com/martin/network-service/internal/domain"
	"github.com/martin/network-service/pkg/logger"
	"go.uber.org/zap"
)

func (s *networkService) AttachResourceToVPC(ctx context.Context, tenantID, resourceARN, vpcID string) error {

	l := logger.WithContext(ctx)

	l.Info("Attaching resource to VPC",
		zap.String("resource_arn", resourceARN),
		zap.String("vpc_id", vpcID),
	)

	valid, status, reason, err := s.ValidateVPC(ctx, tenantID, vpcID)

	if err != nil {
		return err
	}

	if !valid {
		return fmt.Errorf("VPC validation failed (%s): %s", status, reason)
	}

	currentVPC, err := s.resourceRepo.GetByResource(ctx, resourceARN)

	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if currentVPC == vpcID {
		l.Info("Resource already attached to VPC")
		return nil
	}

	if currentVPC != "" {
		l.Info("Moving resource to new VPC",
			zap.String("old_vpc", currentVPC),
			zap.String("new_vpc", vpcID),
		)
	}

	err = s.resourceRepo.Assign(ctx, tenantID, resourceARN, vpcID)

	if err != nil {
		return err
	}

	l.Info("Resource attached to VPC successfully")

	return nil
}

func (s *networkService) DetachResourceFromVPC(ctx context.Context, resourceARN string) error {

	l := logger.WithContext(ctx)

	l.Info("Detaching resource from VPC",
		zap.String("resource_arn", resourceARN),
	)

	return s.resourceRepo.Detach(ctx, resourceARN)
}

func (s *networkService) AttachResourceToSubnet(
	ctx context.Context,
	tenantID,
	resourceARN,
	vpcID,
	subnetID string,
) (string, error) {

	l := logger.WithContext(ctx).With(
		zap.String("tenant_id", tenantID),
		zap.String("resource_arn", resourceARN),
		zap.String("vpc_id", vpcID),
		zap.String("subnet_id", subnetID),
	)

	l.Info("Attaching resource to subnet")

	// Validate VPC
	valid, status, reason, err := s.ValidateVPC(ctx, tenantID, vpcID)

	if err != nil {
		return "", err
	}

	if !valid {
		return "", fmt.Errorf("VPC validation failed (%s): %s", status, reason)
	}

	// Validate subnet
	subnet, err := s.subnetRepo.GetByID(ctx, subnetID)

	if err != nil {

		if err == sql.ErrNoRows {
			return "", fmt.Errorf("subnet not found: %s", subnetID)
		}

		return "", err
	}

	if subnet.VPCID != vpcID {
		return "", fmt.Errorf("subnet does not belong to VPC")
	}

	// Check existing assignment
	existing, err := s.netAssignRepo.GetByResource(ctx, resourceARN)

	if err == nil && existing != nil && existing.PrivateIP != "" {

		l.Info("Resource already has assigned IP",
			zap.String("private_ip", existing.PrivateIP),
		)

		return existing.PrivateIP, nil
	}

	// Allocate new IP
	privateIP, err := s.allocateIP(ctx, subnet.CIDRBlock, subnetID)

	if err != nil {

		l.Error("Failed to allocate IP",
			zap.Error(err),
		)

		return "", err
	}

	l.Info("Allocated private IP",
		zap.String("private_ip", privateIP),
	)

	// Persist assignment
	err = s.netAssignRepo.AssignResource(
		ctx,
		tenantID,
		resourceARN,
		vpcID,
		subnetID,
		privateIP,
	)

	if err != nil {
		return "", fmt.Errorf("failed to store assignment: %w", err)
	}

	l.Info("Resource attached successfully",
		zap.String("private_ip", privateIP),
	)

	return privateIP, nil
}

func (s *networkService) allocateIP(
	ctx context.Context,
	cidr string,
	subnetID string,
) (string, error) {

	ip, ipNet, err := net.ParseCIDR(cidr)

	if err != nil {
		return "", fmt.Errorf("invalid CIDR %s", cidr)
	}

	assignments, err := s.netAssignRepo.ListResourcesInSubnet(ctx, subnetID)

	if err != nil {
		return "", err
	}

	used := map[string]bool{}

	for _, a := range assignments {

		if a.PrivateIP != "" {
			used[a.PrivateIP] = true
		}
	}

	start := cloneIP(ip.Mask(ipNet.Mask))

	// skip .0
	incrementIP(start)

	// skip gateway .1
	incrementIP(start)

	cur := cloneIP(start)

	for ipNet.Contains(cur) {

		ipStr := cur.String()

		next := cloneIP(cur)

		incrementIP(next)

		// broadcast address detection
		if !ipNet.Contains(next) {
			break
		}

		if !used[ipStr] {
			return ipStr, nil
		}

		incrementIP(cur)
	}

	return "", fmt.Errorf("no free IPs in subnet %s", cidr)
}

func cloneIP(ip net.IP) net.IP {

	out := make(net.IP, len(ip))

	copy(out, ip)

	return out
}

func incrementIP(ip net.IP) {

	for i := len(ip) - 1; i >= 0; i-- {

		ip[i]++

		if ip[i] != 0 {
			break
		}
	}
}

func (s *networkService) DetachResourceFromNetwork(ctx context.Context, resourceARN string) error {

	l := logger.WithContext(ctx).With(
		zap.String("resource_arn", resourceARN),
	)

	l.Info("Detaching resource from network")

	_, err := s.netAssignRepo.GetByResource(ctx, resourceARN)

	if err == sql.ErrNoRows {
		return nil
	}

	if err != nil {
		return err
	}

	return s.netAssignRepo.DetachResource(ctx, resourceARN)
}

func (s *networkService) ListResourcesInVPC(
	ctx context.Context,
	tenantID,
	vpcID string,
) ([]domain.ResourceNetworkAssignment, error) {

	l := logger.WithContext(ctx).With(
		zap.String("tenant_id", tenantID),
		zap.String("vpc_id", vpcID),
	)

	valid, _, reason, err := s.ValidateVPC(ctx, tenantID, vpcID)

	if err != nil {
		return nil, err
	}

	if !valid {
		return nil, fmt.Errorf("VPC verification failed: %s", reason)
	}

	assignments, err := s.netAssignRepo.ListResourcesInVPC(ctx, vpcID)

	if err != nil {
		return nil, err
	}

	l.Info("Listed resources in VPC",
		zap.Int("count", len(assignments)),
	)

	return assignments, nil
}

func (s *networkService) ResolveResourceNetwork(
	ctx context.Context,
	resourceARN string,
) (*domain.ResourceNetworkAssignment, error) {

	l := logger.WithContext(ctx).With(
		zap.String("resource_arn", resourceARN),
	)

	assignment, err := s.netAssignRepo.GetByResource(ctx, resourceARN)

	if err != nil {

		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("network assignment not found")
		}

		return nil, err
	}

	l.Info("Resolved resource network",
		zap.String("vpc_id", assignment.VPCID),
		zap.String("subnet_id", assignment.SubnetID),
		zap.String("private_ip", assignment.PrivateIP),
	)

	return assignment, nil
}
