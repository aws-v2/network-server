package driver

import (
	"github.com/stretchr/testify/mock"
)

type MockBridgeDriver struct {
	mock.Mock
}

func (m *MockBridgeDriver) CreateBridge(name string) error {
	args := m.Called(name)
	return args.Error(0)
}

func (m *MockBridgeDriver) DeleteBridge(name string) error {
	args := m.Called(name)
	return args.Error(0)
}

func (m *MockBridgeDriver) Exists(name string) (bool, error) {
	args := m.Called(name)
	return args.Bool(0), args.Error(1)
}

func (m *MockBridgeDriver) AssignIP(name, cidr string) error {
	args := m.Called(name, cidr)
	return args.Error(0)
}

func (m *MockBridgeDriver) BringUp(name string) error {
	args := m.Called(name)
	return args.Error(0)
}

func (m *MockBridgeDriver) AddIPAlias(ip, device string) error {
	args := m.Called(ip, device)
	return args.Error(0)
}

func (m *MockBridgeDriver) RemoveIPAlias(ip, device string) error {
	args := m.Called(ip, device)
	return args.Error(0)
}

type MockIptablesDriver struct {
	mock.Mock
}

func (m *MockIptablesDriver) SetupMasquerade(cidr string, bridgeName string) error {
	args := m.Called(cidr, bridgeName)
	return args.Error(0)
}

func (m *MockIptablesDriver) SetupDNAT(publicIP, privateIP string) error {
	args := m.Called(publicIP, privateIP)
	return args.Error(0)
}

func (m *MockIptablesDriver) SetupSNAT(privateIP, publicIP string) error {
	args := m.Called(privateIP, publicIP)
	return args.Error(0)
}

func (m *MockIptablesDriver) RemoveDNAT(publicIP, privateIP string) error {
	args := m.Called(publicIP, privateIP)
	return args.Error(0)
}

func (m *MockIptablesDriver) RemoveSNAT(privateIP, publicIP string) error {
	args := m.Called(privateIP, publicIP)
	return args.Error(0)
}

func (m *MockIptablesDriver) SetupRDSDDNAT(publicIP string, publicPort int, privateIP string, privatePort int) error {
	args := m.Called(publicIP, publicPort, privateIP, privatePort)
	return args.Error(0)
}

func (m *MockIptablesDriver) SetupRDSDNAT(publicIP string, publicPort int, privateIP string, privatePort int) error {
	args := m.Called(publicIP, publicPort, privateIP, privatePort)
	return args.Error(0)
}

func (m *MockIptablesDriver) SetupRDSSNAT(privateIP string, privatePort int, publicIP string, publicPort int) error {
	args := m.Called(privateIP, privatePort, publicIP, publicPort)
	return args.Error(0)
}

func (m *MockIptablesDriver) RemoveRDSDNAT(publicIP string, publicPort int, privateIP string, privatePort int) error {
	args := m.Called(publicIP, publicPort, privateIP, privatePort)
	return args.Error(0)
}

func (m *MockIptablesDriver) RemoveRDSSNAT(privateIP string, privatePort int, publicIP string, publicPort int) error {
	args := m.Called(privateIP, privatePort, publicIP, publicPort)
	return args.Error(0)
}

type MockDockerNetworkDriver struct {
	mock.Mock
}

func (m *MockDockerNetworkDriver) RegisterBridge(bridgeName, subnet, gateway string) error {
	args := m.Called(bridgeName, subnet, gateway)
	return args.Error(0)
}

func (m *MockDockerNetworkDriver) UnregisterBridge(bridgeName string) error {
	args := m.Called(bridgeName)
	return args.Error(0)
}

func (m *MockDockerNetworkDriver) BridgeExists(bridgeName string) (bool, error) {
	args := m.Called(bridgeName)
	return args.Bool(0), args.Error(1)
}
