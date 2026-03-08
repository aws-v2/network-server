package driver

import (
	"fmt"
	"os/exec"

	"go.uber.org/zap"
)

type DockerNetworkDriver interface {
	RegisterBridge(bridgeName, subnet, gateway string) error
	UnregisterBridge(bridgeName string) error
	BridgeExists(bridgeName string) (bool, error)
}

type dockerNetworkDriver struct{}

func NewDockerNetworkDriver() DockerNetworkDriver {
	return &dockerNetworkDriver{}
}

func (d *dockerNetworkDriver) BridgeExists(bridgeName string) (bool, error) {
	cmd := exec.Command("docker", "network", "inspect", bridgeName)
	err := cmd.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			// docker network inspect returns non-zero when network is not found
			return false, nil
		}
		// Some other unexpected error
		return false, fmt.Errorf("failed to check if docker network %s exists: %w", bridgeName, err)
	}
	return true, nil
}

func (d *dockerNetworkDriver) RegisterBridge(bridgeName, subnet, gateway string) error {
	l := zap.L().With(zap.String("bridge", bridgeName))

	exists, err := d.BridgeExists(bridgeName)
	if err != nil {
		return fmt.Errorf("failed during BridgeExists check: %w", err)
	}
	if exists {
		l.Info("Docker network already exists, skipping registration")
		return nil
	}

	l.Info("Registering bridge with Docker",
		zap.String("subnet", subnet),
		zap.String("gateway", gateway),
	)

	// docker network create \
	//   --driver bridge \
	//   --attachable \
	//   --subnet <subnet> \
	//   --gateway <gateway> \
	//   -o "com.docker.network.bridge.name=<bridgeName>" \
	//   <bridgeName>
	cmd := exec.Command("docker", "network", "create",
		"--driver", "bridge",
		"--attachable",
		"--subnet", subnet,
		"--gateway", gateway,
		"-o", fmt.Sprintf("com.docker.network.bridge.name=%s", bridgeName),
		bridgeName,
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to register docker network %s: %w (output: %s)", bridgeName, err, string(out))
	}

	l.Info("Successfully registered bridge with Docker")
	return nil
}

func (d *dockerNetworkDriver) UnregisterBridge(bridgeName string) error {
	l := zap.L().With(zap.String("bridge", bridgeName))
	l.Info("Unregistering bridge from Docker")

	cmd := exec.Command("docker", "network", "rm", bridgeName)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Ignore error if network not found
		if _, ok := err.(*exec.ExitError); ok {
			l.Debug("Docker network already removed or not found")
			return nil
		}
		return fmt.Errorf("failed to unregister docker network %s: %w (output: %s)", bridgeName, err, string(out))
	}

	return nil
}
