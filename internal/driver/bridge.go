package driver

import (
	"fmt"
	"os/exec"
	"strings"

	"go.uber.org/zap"
)

type BridgeDriver interface {
	CreateBridge(name string) error
	DeleteBridge(name string) error
	Exists(name string) (bool, error)
	AssignIP(name, cidr string) error
	BringUp(name string) error
	AddIPAlias(ip, device string) error
	RemoveIPAlias(ip, device string) error
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
		// Ignore if the address is already assigned
		if strings.Contains(string(out), "File exists") {
			return nil
		}
		return fmt.Errorf("failed to assign IP to bridge %s: %w (output: %s)", name, err, string(out))
	}

	return nil
}

func (d *linuxBridgeDriver) BringUp(name string) error {
	zap.L().Info("Bringing up interface", zap.String("device", name))
	cmd := exec.Command("ip", "link", "set", "dev", name, "up")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set interface %s up: %w (output: %s)", name, err, string(out))
	}
	return nil
}

func (d *linuxBridgeDriver) AddIPAlias(ip, device string) error {
	zap.L().Info("Adding IP alias to device", zap.String("ip", ip), zap.String("device", device))

	// ip addr add <ip>/32 dev <device>
	cmd := exec.Command("ip", "addr", "add", ip+"/32", "dev", device)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add IP alias %s to %s: %w (output: %s)", ip, device, err, string(out))
	}

	return nil
}

func (d *linuxBridgeDriver) RemoveIPAlias(ip, device string) error {
	zap.L().Info("Removing IP alias from device", zap.String("ip", ip), zap.String("device", device))

	// ip addr del <ip>/32 dev <device>
	cmd := exec.Command("ip", "addr", "del", ip+"/32", "dev", device)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to remove IP alias %s from %s: %w (output: %s)", ip, device, err, string(out))
	}

	return nil
}
