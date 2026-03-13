package service_test

import (
	"context"
	"testing"

	"github.com/martin/network-service/internal/domain"
	"github.com/martin/network-service/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestReconcileVPCs_HappyPath(t *testing.T) {
	// Setup mocks
	svc, mocks := service.NewTestService()
	ctx := context.Background()

	// Data
	vpc := domain.VPC{
		ID:         "vpc-1",
		BridgeName: "br-vpc-1",
		Status:     domain.VPCStatusProvisioning,
	}
	subnets := []domain.Subnet{
		{
			ID:        "subnet-1",
			Name:      "public-subnet",
			CIDRBlock: "10.0.1.0/24",
		},
	}

	// Expectations
	mocks.VPCRepo.On("ListAll", ctx).Return([]domain.VPC{vpc}, nil)
	mocks.BridgeDriver.On("Exists", vpc.BridgeName).Return(false, nil) // Bridge missing
	mocks.BridgeDriver.On("CreateBridge", vpc.BridgeName).Return(nil)
	mocks.BridgeDriver.On("BringUp", vpc.BridgeName).Return(nil)

	mocks.SubnetRepo.On("ListByVPC", ctx, vpc.ID).Return(subnets, nil)
	mocks.BridgeDriver.On("AssignIP", vpc.BridgeName, "10.0.1.1/24").Return(nil)
	mocks.IptablesDriver.On("SetupMasquerade", "10.0.1.0/24").Return(nil)
	mocks.DockerNetworkDriver.On("RegisterBridge", vpc.BridgeName, "10.0.1.0/24", "10.0.1.1").Return(nil)

	mocks.VPCRepo.On("UpdateStatus", ctx, vpc.ID, domain.VPCStatusActive).Return(nil)

	// Execute
	err := svc.ReconcileVPCs(ctx)

	// Verify
	assert.NoError(t, err)
	mocks.VPCRepo.AssertExpectations(t)
	mocks.BridgeDriver.AssertExpectations(t)
	mocks.SubnetRepo.AssertExpectations(t)
	mocks.IptablesDriver.AssertExpectations(t)
	mocks.DockerNetworkDriver.AssertExpectations(t)
}

func TestReconcileVPCs_BridgeExists(t *testing.T) {
	// Setup mocks
	svc, mocks := service.NewTestService()
	ctx := context.Background()

	vpc := domain.VPC{ID: "vpc-1", BridgeName: "br-vpc-1", Status: domain.VPCStatusActive}

	mocks.VPCRepo.On("ListAll", ctx).Return([]domain.VPC{vpc}, nil)
	mocks.BridgeDriver.On("Exists", vpc.BridgeName).Return(true, nil) // Bridge already exists
	// CreateBridge should NOT be called
	mocks.BridgeDriver.On("BringUp", vpc.BridgeName).Return(nil)

	mocks.SubnetRepo.On("ListByVPC", ctx, vpc.ID).Return([]domain.Subnet{}, nil)
	mocks.VPCRepo.On("UpdateStatus", ctx, vpc.ID, domain.VPCStatusActive).Return(nil)

	// Execute
	err := svc.ReconcileVPCs(ctx)

	// Verify
	assert.NoError(t, err)
	mocks.BridgeDriver.AssertNotCalled(t, "CreateBridge", mock.Anything)
	mock.AssertExpectationsForObjects(t, mocks.VPCRepo, mocks.BridgeDriver)
}

func TestReconcileVPCs_BridgeCreationFailure(t *testing.T) {
	// Setup mocks
	svc, mocks := service.NewTestService()
	ctx := context.Background()

	vpc := domain.VPC{ID: "vpc-1", BridgeName: "br-vpc-1", Status: domain.VPCStatusProvisioning}

	mocks.VPCRepo.On("ListAll", ctx).Return([]domain.VPC{vpc}, nil)
	mocks.BridgeDriver.On("Exists", vpc.BridgeName).Return(false, nil)
	mocks.BridgeDriver.On("CreateBridge", vpc.BridgeName).Return(assert.AnError) // Forced failure

	// Expect status to be updated to ERROR
	mocks.VPCRepo.On("UpdateStatus", ctx, vpc.ID, domain.VPCStatusError).Return(nil)

	// Execute
	err := svc.ReconcileVPCs(ctx)

	// Verify - ReconcileVPCs handles internal errors per VPC and continues, so it returns nil but logs errors
	assert.NoError(t, err)
	mock.AssertExpectationsForObjects(t, mocks.VPCRepo, mocks.BridgeDriver)
}
