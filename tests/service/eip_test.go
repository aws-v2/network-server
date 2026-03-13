package service_test

import (
	"context"
	"testing"

	"github.com/martin/network-service/internal/domain"
	"github.com/martin/network-service/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestAllocateEIP(t *testing.T) {
	svc, mocks := service.NewTestService()
	ctx := context.Background()

	mocks.EIPRepo.On("Create", ctx, mock.MatchedBy(func(eip *domain.ElasticIP) bool {
		return eip.PublicIP != "" && !eip.Allocated
	})).Return(nil)

	eip, err := svc.AllocateEIP(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, eip)
	assert.True(t, len(eip.PublicIP) > 0)
	mocks.EIPRepo.AssertExpectations(t)
}

func TestAssociateEIP_Success(t *testing.T) {
	svc, mocks := service.NewTestService()
	ctx := context.Background()
	eipID := "eip-1"
	instanceID := "inst-1"
	privateIP := "10.0.1.50"
	publicIP := "203.0.113.1"

	eip := &domain.ElasticIP{
		ID:       eipID,
		PublicIP: publicIP,
	}

	// 1. Get existing EIP
	mocks.EIPRepo.On("GetByID", ctx, eipID).Return(eip, nil)

	// 2. Transaction
	mocks.DBMock.ExpectBegin()
	mocks.EIPRepo.On("UpdateWithTx", ctx, mock.Anything, mock.MatchedBy(func(e *domain.ElasticIP) bool {
		return e.Allocated && e.InstanceID == instanceID && e.PrivateIP == privateIP
	})).Return(nil)
	mocks.DBMock.ExpectCommit()

	// 3. Driver calls
	mocks.BridgeDriver.On("AddIPAlias", publicIP, mock.Anything).Return(nil)
	mocks.IptablesDriver.On("SetupDNAT", publicIP, privateIP).Return(nil)
	mocks.IptablesDriver.On("SetupSNAT", privateIP, publicIP).Return(nil)

	err := svc.AssociateEIP(ctx, eipID, instanceID, privateIP)

	assert.NoError(t, err)
	assert.NoError(t, mocks.DBMock.ExpectationsWereMet())
	mocks.EIPRepo.AssertExpectations(t)
	mocks.BridgeDriver.AssertExpectations(t)
	mocks.IptablesDriver.AssertExpectations(t)
}

func TestDisassociateEIP_Success(t *testing.T) {
	svc, mocks := service.NewTestService()
	ctx := context.Background()
	eipID := "eip-1"
	publicIP := "203.0.113.1"
	privateIP := "10.0.1.50"

	eip := &domain.ElasticIP{
		ID:         eipID,
		PublicIP:   publicIP,
		PrivateIP:  privateIP,
		InstanceID: "inst-1",
		Allocated:  true,
	}

	mocks.EIPRepo.On("GetByID", ctx, eipID).Return(eip, nil)

	mocks.DBMock.ExpectBegin()
	mocks.EIPRepo.On("UpdateWithTx", ctx, mock.Anything, mock.MatchedBy(func(e *domain.ElasticIP) bool {
		return !e.Allocated && e.InstanceID == "" && e.PrivateIP == ""
	})).Return(nil)
	mocks.DBMock.ExpectCommit()

	mocks.BridgeDriver.On("RemoveIPAlias", publicIP, mock.Anything).Return(nil)
	mocks.IptablesDriver.On("RemoveDNAT", publicIP, privateIP).Return(nil)
	mocks.IptablesDriver.On("RemoveSNAT", privateIP, publicIP).Return(nil)

	err := svc.DisassociateEIP(ctx, eipID)

	assert.NoError(t, err)
	assert.NoError(t, mocks.DBMock.ExpectationsWereMet())
}
