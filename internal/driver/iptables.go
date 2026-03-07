package driver

import (
	"fmt"
	"os/exec"
	"runtime"

	"go.uber.org/zap"
)

type IptablesDriver interface {
	SetupMasquerade(cidr string) error
	SetupDNAT(publicIP, privateIP string) error
	SetupSNAT(privateIP, publicIP string) error
	RemoveDNAT(publicIP, privateIP string) error
	RemoveSNAT(privateIP, publicIP string) error

	// Port-specific NAT for RDS Database Containers
	SetupRDSDNAT(publicIP string, publicPort int, privateIP string, privatePort int) error
	SetupRDSSNAT(privateIP string, privatePort int, publicIP string, publicPort int) error
	RemoveRDSDNAT(publicIP string, publicPort int, privateIP string, privatePort int) error
	RemoveRDSSNAT(privateIP string, privatePort int, publicIP string, publicPort int) error
}

type linuxIptablesDriver struct{}

func NewIptablesDriver() IptablesDriver {
	return &linuxIptablesDriver{}
}

func (d *linuxIptablesDriver) SetupMasquerade(cidr string) error {
	if runtime.GOOS != "linux" {
		zap.L().Warn("Skipping iptables MASQUERADE: not on Linux", zap.String("cidr", cidr))
		return nil
	}

	// Check if rule exists: iptables -t nat -C POSTROUTING -s <cidr> -j MASQUERADE
	exists, _ := d.ruleExists("nat", "POSTROUTING", "-s", cidr, "-j", MASQUERADE)
	if exists {
		return nil
	}

	zap.L().Info("Adding iptables MASQUERADE rule", zap.String("cidr", cidr))
	cmd := exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", cidr, "-j", "MASQUERADE")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add MASQUERADE rule: %w (output: %s)", err, string(out))
	}

	return nil
}

func (d *linuxIptablesDriver) SetupDNAT(publicIP, privateIP string) error {
	if runtime.GOOS != "linux" {
		zap.L().Warn("Skipping DNAT: not on Linux", zap.String("public_ip", publicIP), zap.String("private_ip", privateIP))
		return nil
	}

	exists, _ := d.ruleExists("nat", "PREROUTING", "-d", publicIP, "-j", "DNAT", "--to-destination="+privateIP)
	if exists {
		return nil
	}

	zap.L().Info("Adding DNAT rule", zap.String("public_ip", publicIP), zap.String("private_ip", privateIP))
	cmd := exec.Command("iptables", "-t", "nat", "-A", "PREROUTING", "-d", publicIP, "-j", "DNAT", "--to-destination="+privateIP)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add DNAT rule: %w (output: %s)", err, string(out))
	}
	return nil
}

func (d *linuxIptablesDriver) SetupSNAT(privateIP, publicIP string) error {
	if runtime.GOOS != "linux" {
		zap.L().Warn("Skipping SNAT: not on Linux", zap.String("private_ip", privateIP), zap.String("public_ip", publicIP))
		return nil
	}

	exists, _ := d.ruleExists("nat", "POSTROUTING", "-s", privateIP, "-j", "SNAT", "--to-source="+publicIP)
	if exists {
		return nil
	}

	zap.L().Info("Adding SNAT rule", zap.String("private_ip", privateIP), zap.String("public_ip", publicIP))
	cmd := exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", privateIP, "-j", "SNAT", "--to-source="+publicIP)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add SNAT rule: %w (output: %s)", err, string(out))
	}
	return nil
}

func (d *linuxIptablesDriver) RemoveDNAT(publicIP, privateIP string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	zap.L().Info("Removing DNAT rule", zap.String("public_ip", publicIP), zap.String("private_ip", privateIP))
	cmd := exec.Command("iptables", "-t", "nat", "-D", "PREROUTING", "-d", publicIP, "-j", "DNAT", "--to-destination="+privateIP)
	_ = cmd.Run() // Ignore error if rule doesn't exist
	return nil
}

func (d *linuxIptablesDriver) RemoveSNAT(privateIP, publicIP string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	zap.L().Info("Removing SNAT rule", zap.String("private_ip", privateIP), zap.String("public_ip", publicIP))
	cmd := exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING", "-s", privateIP, "-j", "SNAT", "--to-source="+publicIP)
	_ = cmd.Run() // Ignore error if rule doesn't exist
	return nil
}

func (d *linuxIptablesDriver) SetupRDSDNAT(publicIP string, publicPort int, privateIP string, privatePort int) error {
	if runtime.GOOS != "linux" {
		zap.L().Warn("Skipping DNAT: not on Linux", zap.String("public_ip", publicIP), zap.Int("public_port", publicPort), zap.String("private_ip", privateIP), zap.Int("private_port", privatePort))
		return nil
	}

	dest := fmt.Sprintf("--to-destination=%s:%d", privateIP, privatePort)
	port := fmt.Sprint(publicPort)

	// PREROUTING — handles traffic from external machines
	exists, _ := d.ruleExists("nat", "PREROUTING", "-p", "tcp", "-d", publicIP, "--dport", port, "-j", "DNAT", dest)
	if !exists {
		zap.L().Info(fmt.Sprintf("[IPTABLES] Adding RDS DNAT rule (PREROUTING): %s:%d -> %s:%d", publicIP, publicPort, privateIP, privatePort))
		cmd := exec.Command("iptables", "-t", "nat", "-A", "PREROUTING", "-p", "tcp", "-d", publicIP, "--dport", port, "-j", "DNAT", dest)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to add RDS DNAT PREROUTING rule: %w (output: %s)", err, string(out))
		}
	}

	// OUTPUT — handles traffic originating from this machine (local psql, curl, etc.)
	exists, _ = d.ruleExists("nat", "OUTPUT", "-p", "tcp", "-d", publicIP, "--dport", port, "-j", "DNAT", dest)
	if !exists {
		zap.L().Info(fmt.Sprintf("[IPTABLES] Adding RDS DNAT rule (OUTPUT): %s:%d -> %s:%d", publicIP, publicPort, privateIP, privatePort))
		cmd := exec.Command("iptables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "-d", publicIP, "--dport", port, "-j", "DNAT", dest)
		if _, err := cmd.CombinedOutput(); err != nil {
			zap.L().Warn("[IPTABLES] Failed to add RDS DNAT OUTPUT rule (non-fatal)", zap.Error(err))
		}
	}

	return nil
}

func (d *linuxIptablesDriver) SetupRDSSNAT(privateIP string, privatePort int, publicIP string, publicPort int) error {
	if runtime.GOOS != "linux" {
		zap.L().Warn("Skipping SNAT: not on Linux", zap.String("private_ip", privateIP), zap.Int("private_port", privatePort), zap.String("public_ip", publicIP), zap.Int("public_port", publicPort))
		return nil
	}

	exists, _ := d.ruleExists("nat", "POSTROUTING", "-p", "tcp", "-s", privateIP, "--sport", fmt.Sprint(privatePort), "-j", "SNAT", fmt.Sprintf("--to-source=%s:%d", publicIP, publicPort))
	if exists {
		return nil
	}

	zap.L().Info(fmt.Sprintf("[IPTABLES] Adding RDS SNAT rule: %s:%d -> %s:%d", privateIP, privatePort, publicIP, publicPort))
	cmd := exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-p", "tcp", "-s", privateIP, "--sport", fmt.Sprint(privatePort), "-j", "SNAT", fmt.Sprintf("--to-source=%s:%d", publicIP, publicPort))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add RDS SNAT rule: %w (output: %s)", err, string(out))
	}
	return nil
}

func (d *linuxIptablesDriver) RemoveRDSDNAT(publicIP string, publicPort int, privateIP string, privatePort int) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	zap.L().Info("[IPTABLES] Removing RDS DNAT rule")
	dest := fmt.Sprintf("--to-destination=%s:%d", privateIP, privatePort)
	port := fmt.Sprint(publicPort)

	// Remove from PREROUTING
	cmd := exec.Command("iptables", "-t", "nat", "-D", "PREROUTING", "-p", "tcp", "-d", publicIP, "--dport", port, "-j", "DNAT", dest)
	_ = cmd.Run()

	// Remove from OUTPUT
	cmd = exec.Command("iptables", "-t", "nat", "-D", "OUTPUT", "-p", "tcp", "-d", publicIP, "--dport", port, "-j", "DNAT", dest)
	_ = cmd.Run()

	return nil
}

func (d *linuxIptablesDriver) RemoveRDSSNAT(privateIP string, privatePort int, publicIP string, publicPort int) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	zap.L().Info("[IPTABLES] Removing RDS SNAT rule")
	cmd := exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING", "-p", "tcp", "-s", privateIP, "--sport", fmt.Sprint(privatePort), "-j", "SNAT", fmt.Sprintf("--to-source=%s:%d", publicIP, publicPort))
	_ = cmd.Run() // Ignore error if rule doesn't exist
	return nil
}

func (d *linuxIptablesDriver) ruleExists(table, chain string, args ...string) (bool, error) {
	fullArgs := append([]string{"-t", table, "-C", chain}, args...)
	cmd := exec.Command("iptables", fullArgs...)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

const MASQUERADE = "MASQUERADE"
