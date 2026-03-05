package driver

import (
	"fmt"
	"os/exec"

	"go.uber.org/zap"
)

type BridgeDriver interface {
	CreateBridge(name string) error
	DeleteBridge(name string) error
	Exists(name string) (bool, error)
	AssignIP(name, cidr string) error
}

type linuxBridgeDriver struct{}

func NewBridgeDriver() BridgeDriver {
	return &linuxBridgeDriver{}
}

func (d *linuxBridgeDriver) CreateBridge(name string) error {
	exists, err := d.Exists(name)
	if err != nil {
		return err
	}
	if exists {
		zap.L().Info("Bridge already exists, skipping creation", zap.String("bridge", name))
		return nil
	}

	zap.L().Info("Creating Linux bridge", zap.String("bridge", name))

	// ip link add name <name> type bridge
	cmd := exec.Command("ip", "link", "add", "name", name, "type", "bridge")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create bridge %s: %w (output: %s)", name, err, string(out))
	}

	// ip link set dev <name> up
	cmd = exec.Command("ip", "link", "set", "dev", name, "up")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set bridge %s up: %w (output: %s)", name, err, string(out))
	}

	return nil
}

func (d *linuxBridgeDriver) DeleteBridge(name string) error {
	zap.L().Info("Deleting Linux bridge", zap.String("bridge", name))
	cmd := exec.Command("ip", "link", "del", "name", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete bridge %s: %w (output: %s)", name, err, string(out))
	}

	return nil
}

func (d *linuxBridgeDriver) Exists(name string) (bool, error) {
	cmd := exec.Command("ip", "link", "show", name)
	err := cmd.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (d *linuxBridgeDriver) AssignIP(name, cidr string) error {
	zap.L().Info("Assigning IP to bridge", zap.String("bridge", name), zap.String("cidr", cidr))

	// ip addr add <cidr> dev <name>
	cmd := exec.Command("ip", "addr", "add", cidr, "dev", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Ignore if the address already exists (exit code 2 usually, output contains "File exists")
		// Wait, sometimes it's better to just return an error but maybe log it.
		// Actually, let's just return the error if it fails.
		return fmt.Errorf("failed to assign IP to bridge %s: %w (output: %s)", name, err, string(out))
	}

	return nil
}
