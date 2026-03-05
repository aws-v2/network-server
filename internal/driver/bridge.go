package driver

import (
	"fmt"
	"os/exec"
	"runtime"

	"go.uber.org/zap"
)

type BridgeDriver interface {
	CreateBridge(name string) error
	DeleteBridge(name string) error
	Exists(name string) (bool, error)
}

type linuxBridgeDriver struct{}

func NewBridgeDriver() BridgeDriver {
	return &linuxBridgeDriver{}
}

func (d *linuxBridgeDriver) CreateBridge(name string) error {
	if runtime.GOOS != "linux" {
		zap.L().Warn("Skipping bridge creation: not on Linux", zap.String("bridge", name))
		return nil
	}

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
	if runtime.GOOS != "linux" {
		zap.L().Warn("Skipping bridge deletion: not on Linux", zap.String("bridge", name))
		return nil
	}

	zap.L().Info("Deleting Linux bridge", zap.String("bridge", name))
	cmd := exec.Command("ip", "link", "del", "name", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete bridge %s: %w (output: %s)", name, err, string(out))
	}

	return nil
}

func (d *linuxBridgeDriver) Exists(name string) (bool, error) {
	if runtime.GOOS != "linux" {
		return false, nil
	}

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
