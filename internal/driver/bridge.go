package driver

import (
	"fmt"
	"os/exec"
	"strings"

	"go.uber.org/zap"
)

// hostCmd runs an ip command in the host network namespace via nsenter.
// Requires the container to be started with --pid=host and --privileged (or NET_ADMIN).
func hostCmd(args ...string) *exec.Cmd {
	// nsenter -t 1 -n -- <args...>
	// -t 1  : target PID 1 (init), which always lives in the host namespace
	// -n    : enter the network namespace of that PID
	full := append([]string{"-t", "1", "-n", "--"}, args...)
	return exec.Command("nsenter", full...)
}

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

	zap.L().Info("Creating Linux bridge on host ns", zap.String("bridge", name))

	if out, err := hostCmd("ip", "link", "add", "name", name, "type", "bridge").CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create bridge %s: %w (output: %s)", name, err, string(out))
	}

	if out, err := hostCmd("ip", "link", "set", "dev", name, "up").CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set bridge %s up: %w (output: %s)", name, err, string(out))
	}

	return nil
}

func (d *linuxBridgeDriver) DeleteBridge(name string) error {
	zap.L().Info("Deleting Linux bridge on host ns", zap.String("bridge", name))
	if out, err := hostCmd("ip", "link", "del", name).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete bridge %s: %w (output: %s)", name, err, string(out))
	}
	return nil
}

func (d *linuxBridgeDriver) Exists(name string) (bool, error) {
	err := hostCmd("ip", "link", "show", name).Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (d *linuxBridgeDriver) AssignIP(name, cidr string) error {
	zap.L().Info("Assigning IP to bridge on host ns", zap.String("bridge", name), zap.String("cidr", cidr))
	if out, err := hostCmd("ip", "addr", "add", cidr, "dev", name).CombinedOutput(); err != nil {
		if strings.Contains(string(out), "File exists") {
			return nil
		}
		return fmt.Errorf("failed to assign IP to bridge %s: %w (output: %s)", name, err, string(out))
	}
	return nil
}

func (d *linuxBridgeDriver) BringUp(name string) error {
	zap.L().Info("Bringing up interface on host ns", zap.String("device", name))
	if out, err := hostCmd("ip", "link", "set", "dev", name, "up").CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set interface %s up: %w (output: %s)", name, err, string(out))
	}
	return nil
}

func (d *linuxBridgeDriver) AddIPAlias(ip, device string) error {
	zap.L().Info("Adding IP alias on host ns", zap.String("ip", ip), zap.String("device", device))
	if out, err := hostCmd("ip", "addr", "add", ip+"/32", "dev", device).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add IP alias %s to %s: %w (output: %s)", ip, device, err, string(out))
	}
	return nil
}

func (d *linuxBridgeDriver) RemoveIPAlias(ip, device string) error {
	zap.L().Info("Removing IP alias on host ns", zap.String("ip", ip), zap.String("device", device))
	if out, err := hostCmd("ip", "addr", "del", ip+"/32", "dev", device).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to remove IP alias %s from %s: %w (output: %s)", ip, device, err, string(out))
	}
	return nil
}