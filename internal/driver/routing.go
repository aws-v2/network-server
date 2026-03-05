package driver

import (
	"fmt"
	"os/exec"

	"go.uber.org/zap"
)

type RoutingDriver interface {
	AddRoute(bridgeName, destination, gateway string) error
	DeleteRoute(bridgeName, destination, gateway string) error
}

type linuxRoutingDriver struct{}

func NewRoutingDriver() RoutingDriver {
	return &linuxRoutingDriver{}
}

func (d *linuxRoutingDriver) AddRoute(bridgeName, destination, gateway string) error {
	// Check if route already exists
	checkCmd := exec.Command("ip", "route", "show", "to", destination, "dev", bridgeName)
	if out, err := checkCmd.Output(); err == nil && len(out) > 0 {
		return nil // Route already exists
	}

	zap.L().Info("Adding system route", zap.String("bridge", bridgeName), zap.String("destination", destination), zap.String("gateway", gateway))

	// ip route add <destination> via <gateway> dev <bridgeName>
	cmd := exec.Command("ip", "route", "add", destination, "via", gateway, "dev", bridgeName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add route: %w (output: %s)", err, string(out))
	}

	return nil
}

func (d *linuxRoutingDriver) DeleteRoute(bridgeName, destination, gateway string) error {
	zap.L().Info("Deleting system route", zap.String("bridge", bridgeName), zap.String("destination", destination))

	cmd := exec.Command("ip", "route", "del", destination, "dev", bridgeName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete route: %w (output: %s)", err, string(out))
	}

	return nil
}
