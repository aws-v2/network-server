package service_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/martin/network-service/internal/domain"
	"github.com/martin/network-service/internal/repository"
	"github.com/martin/network-service/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestListVPCs(t *testing.T) {
	// Setup
	svc, mocks := service.NewTestService()

	tenantID := "tenant-1"
	expectedVpcs := []domain.VPC{
		{ID: "vpc-1", Name: "VPC 1", TenantID: tenantID},
		{ID: "vpc-2", Name: "VPC 2", TenantID: tenantID},
	}

	// Define behavior
	mocks.VPCRepo.On("ListByTenant", mock.Anything, tenantID).Return(expectedVpcs, nil)

	// Execute
	vpcs, err := svc.ListVPCs(context.Background(), tenantID)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, 2, len(vpcs))
	assert.Equal(t, "vpc-1", vpcs[0].ID)
	mocks.VPCRepo.AssertExpectations(t)
}

func TestValidateVPC(t *testing.T) {
	tests := []struct {
		name           string
		tenantID       string
		vpcID          string
		setupMock      func(m *repository.MockVPCRepository)
		expectedValid  bool
		expectedStatus string
		expectedMsg    string
		expectErr      bool
	}{
		{
			name:     "Valid Active VPC",
			tenantID: "t1",
			vpcID:    "v1",
			setupMock: func(m *repository.MockVPCRepository) {
				m.On("GetByID", mock.Anything, "v1").Return(&domain.VPC{
					ID: "v1", TenantID: "t1", Status: domain.VPCStatusActive,
				}, nil)
			},
			expectedValid:  true,
			expectedStatus: domain.VPCStatusActive,
			expectedMsg:    "",
		},
		{
			name:     "VPC Not Found",
			tenantID: "t1",
			vpcID:    "v1",
			setupMock: func(m *repository.MockVPCRepository) {
				m.On("GetByID", mock.Anything, "v1").Return(nil, sql.ErrNoRows)
			},
			expectedValid:  false,
			expectedStatus: "",
			expectedMsg:    "VPC not found",
		},
		{
			name:     "Ownership Mismatch",
			tenantID: "t2",
			vpcID:    "v1",
			setupMock: func(m *repository.MockVPCRepository) {
				m.On("GetByID", mock.Anything, "v1").Return(&domain.VPC{
					ID: "v1", TenantID: "t1", Status: domain.VPCStatusActive,
				}, nil)
			},
			expectedValid:  false,
			expectedStatus: domain.VPCStatusActive,
			expectedMsg:    "VPC does not belong to tenant",
		},
		{
			name:     "VPC In Error Status",
			tenantID: "t1",
			vpcID:    "v1",
			setupMock: func(m *repository.MockVPCRepository) {
				m.On("GetByID", mock.Anything, "v1").Return(&domain.VPC{
					ID: "v1", TenantID: "t1", Status: domain.VPCStatusError,
				}, nil)
			},
			expectedValid:  false,
			expectedStatus: domain.VPCStatusError,
			expectedMsg:    "VPC is in error status (likely due to missing bridge or permissions)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, mocks := service.NewTestService()
			tt.setupMock(mocks.VPCRepo)

			valid, status, msg, err := svc.ValidateVPC(context.Background(), tt.tenantID, tt.vpcID)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedValid, valid)
				assert.Equal(t, tt.expectedStatus, status)
				assert.Equal(t, tt.expectedMsg, msg)
			}
			mocks.VPCRepo.AssertExpectations(t)
		})
	}
}

func TestCreateVPC_DuplicateName(t *testing.T) {
	// Setup
	svc, mocks := service.NewTestService()

	tenantID := "tenant-1"
	vpcName := "existing-vpc"
	existingVpcs := []domain.VPC{
		{ID: "vpc-1", Name: vpcName, TenantID: tenantID},
	}

	// Define behavior: ListByTenant returns one VPC with the same name
	mocks.VPCRepo.On("ListByTenant", mock.Anything, tenantID).Return(existingVpcs, nil)

	// Execute
	vpc, err := svc.CreateVPC(context.Background(), tenantID, vpcName)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, "vpc-1", vpc.ID)
	assert.Equal(t, vpcName, vpc.Name)
	mocks.VPCRepo.AssertExpectations(t)
}

func TestCreateDefaultVPC_AlreadyExists(t *testing.T) {
	// Setup
	svc, mocks := service.NewTestService()

	tenantID := "tenant-1"
	existingVpc := &domain.VPC{ID: "vpc-default", Name: "test-default-vpc", TenantID: tenantID}

	// Define behavior
	mocks.VPCRepo.On("GetDefaultVPC", mock.Anything, tenantID).Return(existingVpc, nil)

	// Execute
	vpc, err := svc.CreateDefaultVPC(context.Background(), tenantID, "test")

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, "vpc-default", vpc.ID)
	mocks.VPCRepo.AssertExpectations(t)
}

func TestCreateVPC_Success(t *testing.T) {
	svc, mocks := service.NewTestService()
	ctx := context.Background()
	tenantID := "tenant-1"
	vpcName := "new-vpc"

	// 1. Initial Checks
	mocks.VPCRepo.On("ListByTenant", ctx, tenantID).Return([]domain.VPC{}, nil)

	// 2. Transaction Setup
	mocks.DBMock.ExpectBegin()

	// 3. Repository Expectations
	mocks.CIDRRepo.On("AllocateNextWithTx", ctx, mock.Anything, tenantID).Return("10.1.0.0/16", nil)
	mocks.VPCRepo.On("CreateWithTx", ctx, mock.Anything, mock.MatchedBy(func(v *domain.VPC) bool {
		return v.Name == vpcName && v.TenantID == tenantID
	})).Return(nil)
	mocks.RTRepo.On("CreateWithTx", ctx, mock.Anything, mock.Anything).Return(nil)
	mocks.SubnetRepo.On("CreateWithTx", ctx, mock.Anything, mock.MatchedBy(func(s *domain.Subnet) bool {
		return s.Name == "public-subnet"
	})).Return(nil)
	mocks.SubnetRepo.On("CreateWithTx", ctx, mock.Anything, mock.MatchedBy(func(s *domain.Subnet) bool {
		return s.Name == "private-subnet"
	})).Return(nil)
	mocks.IGWRepo.On("CreateWithTx", ctx, mock.Anything, mock.Anything).Return(nil)
	mocks.RouteRepo.On("CreateWithTx", ctx, mock.Anything, mock.Anything).Return(nil)
	mocks.SGRepo.On("CreateWithTx", ctx, mock.Anything, mock.Anything).Return(nil)

	// 4. Commit
	mocks.DBMock.ExpectCommit()

	// 5. Driver Expectations (Outside Transaction)
	mocks.BridgeDriver.On("CreateBridge", mock.Anything).Return(nil)
	mocks.DockerNetworkDriver.On("RegisterBridge", mock.Anything, "10.1.1.0/24", "10.1.1.1").Return(nil)
	mocks.BridgeDriver.On("BringUp", mock.Anything).Return(nil)

	// Two subnets exist in this VPC
	mocks.SubnetRepo.On("ListByVPC", ctx, mock.Anything).Return([]domain.Subnet{
		{CIDRBlock: "10.1.1.0/24"},
		{CIDRBlock: "10.1.2.0/24"},
	}, nil)
	mocks.BridgeDriver.On("AssignIP", mock.Anything, "10.1.1.1/24").Return(nil)
	mocks.BridgeDriver.On("AssignIP", mock.Anything, "10.1.2.1/24").Return(nil)

	mocks.IptablesDriver.On("SetupMasquerade", "10.1.1.0/24").Return(nil)
	mocks.VPCRepo.On("UpdateStatus", ctx, mock.Anything, domain.VPCStatusActive).Return(nil)

	// Execute
	vpc, err := svc.CreateVPC(ctx, tenantID, vpcName)

	// Verify
	assert.NoError(t, err)
	assert.NotNil(t, vpc)
	assert.Equal(t, vpcName, vpc.Name)

	assert.NoError(t, mocks.DBMock.ExpectationsWereMet())
	mocks.VPCRepo.AssertExpectations(t)
	mocks.CIDRRepo.AssertExpectations(t)
	mocks.RTRepo.AssertExpectations(t)
	mocks.SubnetRepo.AssertExpectations(t)
	mocks.IGWRepo.AssertExpectations(t)
	mocks.RouteRepo.AssertExpectations(t)
	mocks.SGRepo.AssertExpectations(t)
	mocks.BridgeDriver.AssertExpectations(t)
	mocks.IptablesDriver.AssertExpectations(t)
}
