package service_test

import (
	"context"
	"testing"

	"github.com/martin/network-service/internal/domain"
	"github.com/martin/network-service/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCreateVPC_TransactionFailure(t *testing.T) {
	svc, mocks := service.NewTestService()
	ctx := context.Background()
	tenantID := "tenant-1"
	vpcName := "failed-vpc"

	// 1. Initial Checks
	mocks.VPCRepo.On("ListByTenant", ctx, tenantID).Return([]domain.VPC{}, nil)

	// 2. Transaction begins
	mocks.DBMock.ExpectBegin()

	// 3. First repo call succeeds
	mocks.CIDRRepo.On("AllocateNextWithTx", ctx, mock.Anything, tenantID).Return("10.1.0.0/16", nil)

	// 4. Second repo call FAILS
	mocks.VPCRepo.On("CreateWithTx", ctx, mock.Anything, mock.Anything).Return(assert.AnError)

	// 5. Expect ROLLBACK
	mocks.DBMock.ExpectRollback()

	// Execute
	t.Log("Executing svc.CreateVPC...")
	vpc, err := svc.CreateVPC(ctx, tenantID, vpcName)
	t.Logf("svc.CreateVPC returned vpc=%v, err=%v", vpc, err)

	// Verify
	assert.Error(t, err)
	assert.Nil(t, vpc)
	assert.Contains(t, err.Error(), "failed to create VPC record")

	// Ensure drivers WERE NOT called
	mocks.BridgeDriver.AssertNotCalled(t, "CreateBridge", mock.Anything)

	assert.NoError(t, mocks.DBMock.ExpectationsWereMet())
}

func TestAssociateEIP_DriverFailure(t *testing.T) {
	svc, mocks := service.NewTestService()
	ctx := context.Background()
	eipID := "eip-1"
	instanceID := "inst-1"
	privateIP := "10.0.1.50"
	publicIP := "203.0.113.1"

	eip, mocks2 := service.NewTestService() // Use NewTestService for cleanup
	_ = eip
	_ = mocks2

	// Need to fix this test to use mocks properly
	mocks.EIPRepo.On("GetByID", ctx, eipID).Return(&domain.ElasticIP{ID: eipID, PublicIP: publicIP}, nil)

	// Transaction succeeds
	mocks.DBMock.ExpectBegin()
	mocks.EIPRepo.On("UpdateWithTx", ctx, mock.Anything, mock.Anything).Return(nil)
	mocks.DBMock.ExpectCommit()

	// BridgeDriver FAILS (non-fatal in code, but we check logging/behavior)
	mocks.BridgeDriver.On("AddIPAlias", publicIP, mock.Anything).Return(assert.AnError)

	// Iptables still called (as code proceeds)
	mocks.IptablesDriver.On("SetupDNAT", publicIP, privateIP).Return(nil)
	mocks.IptablesDriver.On("SetupSNAT", privateIP, publicIP).Return(nil)

	err := svc.AssociateEIP(ctx, eipID, instanceID, privateIP)

	// AsssociateEIP returns nil even if drivers fail (it only logs errors)
	assert.NoError(t, err)
	mocks.BridgeDriver.AssertExpectations(t)
}
