package driver

import (
	"fmt"
	"os/exec"
	"runtime"

	"go.uber.org/zap"
)

type IptablesDriver interface {
	SetupMasquerade(cidr string, bridgeName string) error

	SetupPortForward(hostIP string, hostPort int, vmIP string, vmPort int) error
	RemovePortForward(hostIP string, hostPort int, vmIP string, vmPort int) error
}

type linuxIptablesDriver struct{}

func NewIptablesDriver() IptablesDriver {
	return &linuxIptablesDriver{}
}

// -----------------------------
// MASQUERADE (egress from VPC)
// -----------------------------
func (d *linuxIptablesDriver) SetupMasquerade(cidr string, bridgeName string) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	// Enable IP forwarding
	_ = exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run()

	// NAT outgoing traffic
	exists, _ := d.ruleExists("nat", "POSTROUTING", "-s", cidr, "!", "-o", bridgeName, "-j", "MASQUERADE")
	if !exists {
		cmd := exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING",
			"-s", cidr, "!", "-o", bridgeName, "-j", "MASQUERADE")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("MASQUERADE failed: %w (%s)", err, out)
		}
	}

	// Allow forwarding IN
	exists, _ = d.ruleExists("filter", "FORWARD", "-i", bridgeName, "-j", "ACCEPT")
	if !exists {
		exec.Command("iptables", "-A", "FORWARD", "-i", bridgeName, "-j", "ACCEPT").Run()
	}

	// Allow forwarding OUT
	exists, _ = d.ruleExists("filter", "FORWARD", "-o", bridgeName, "-j", "ACCEPT")
	if !exists {
		exec.Command("iptables", "-A", "FORWARD", "-o", bridgeName, "-j", "ACCEPT").Run()
	}

	return nil
}

// -----------------------------
// PORT FORWARD (DNAT + SNAT)
// -----------------------------
func (d *linuxIptablesDriver) SetupPortForward(hostIP string, hostPort int, vmIP string, vmPort int) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	port := fmt.Sprint(hostPort)
	dest := fmt.Sprintf("%s:%d", vmIP, vmPort)

	zap.L().Info("[IPTABLES] Setting up port forward",
		zap.String("host", hostIP),
		zap.String("vm", dest),
	)

	// -----------------------------
	// DNAT (external traffic)
	// -----------------------------
	exists, _ := d.ruleExists("nat", "PREROUTING",
		"-d", hostIP, "-p", "tcp", "--dport", port,
		"-j", "DNAT", "--to-destination", dest)

	if !exists {
		cmd := exec.Command("iptables", "-t", "nat", "-A", "PREROUTING",
			"-d", hostIP, "-p", "tcp", "--dport", port,
			"-j", "DNAT", "--to-destination", dest)

		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("DNAT PREROUTING failed: %w (%s)", err, out)
		}
	}

	// -----------------------------
	// DNAT (LOCAL traffic — IMPORTANT)
	// -----------------------------
	exists, _ = d.ruleExists("nat", "OUTPUT",
		"-d", hostIP, "-p", "tcp", "--dport", port,
		"-j", "DNAT", "--to-destination", dest)

	if !exists {
		cmd := exec.Command("iptables", "-t", "nat", "-A", "OUTPUT",
			"-d", hostIP, "-p", "tcp", "--dport", port,
			"-j", "DNAT", "--to-destination", dest)

		_ = cmd.Run() // not fatal
	}

	// -----------------------------
	// FORWARD allow traffic
	// -----------------------------
	exists, _ = d.ruleExists("filter", "FORWARD",
		"-p", "tcp", "-d", vmIP, "--dport", fmt.Sprint(vmPort),
		"-j", "ACCEPT")

	if !exists {
		exec.Command("iptables", "-A", "FORWARD",
			"-p", "tcp", "-d", vmIP, "--dport", fmt.Sprint(vmPort),
			"-j", "ACCEPT").Run()
	}

	// -----------------------------
	// SNAT (return path)
	// -----------------------------
	exists, _ = d.ruleExists("nat", "POSTROUTING",
		"-p", "tcp", "-d", vmIP, "--dport", fmt.Sprint(vmPort),
		"-j", "MASQUERADE")

	if !exists {
		exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING",
			"-p", "tcp", "-d", vmIP, "--dport", fmt.Sprint(vmPort),
			"-j", "MASQUERADE").Run()
	}

	return nil
}

// -----------------------------
// REMOVE PORT FORWARD
// -----------------------------
func (d *linuxIptablesDriver) RemovePortForward(hostIP string, hostPort int, vmIP string, vmPort int) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	port := fmt.Sprint(hostPort)
	dest := fmt.Sprintf("%s:%d", vmIP, vmPort)

	zap.L().Info("[IPTABLES] Removing port forward",
		zap.String("host", hostIP),
		zap.String("vm", dest),
	)

	exec.Command("iptables", "-t", "nat", "-D", "PREROUTING",
		"-d", hostIP, "-p", "tcp", "--dport", port,
		"-j", "DNAT", "--to-destination", dest).Run()

	exec.Command("iptables", "-t", "nat", "-D", "OUTPUT",
		"-d", hostIP, "-p", "tcp", "--dport", port,
		"-j", "DNAT", "--to-destination", dest).Run()

	exec.Command("iptables", "-D", "FORWARD",
		"-p", "tcp", "-d", vmIP, "--dport", fmt.Sprint(vmPort),
		"-j", "ACCEPT").Run()

	return nil
}

// -----------------------------
// CHECK RULE EXISTS
// -----------------------------
func (d *linuxIptablesDriver) ruleExists(table, chain string, args ...string) (bool, error) {
	fullArgs := append([]string{"-t", table, "-C", chain}, args...)
	cmd := exec.Command("iptables", fullArgs...)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	return false, nil
}