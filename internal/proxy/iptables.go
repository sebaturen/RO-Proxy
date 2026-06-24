package proxy

import (
    "fmt"
    "os"
    "os/exec"
)

const (
    ipsetName = "ro-servers"
)

func InitializeIPTables(targetIPs []string, proxyPort int) error {
    if err := VerifyIPTablesSetup(); err != nil {
        fmt.Fprintf(os.Stderr, "iptables verification failed: %v\n", err)
        fmt.Fprintln(os.Stderr, "Make sure you:")
        fmt.Fprintln(os.Stderr, "  1. Run as root (sudo ./roproxy)")
        fmt.Fprintln(os.Stderr, "  2. Have iptables and ipset installed")
        os.Exit(1)
    }

    if err := SetupIPTables(targetIPs, proxyPort); err != nil {
        fmt.Fprintf(os.Stderr, "Failed to setup iptables: %v\n", err)
        os.Exit(1)
    }

    return nil
}

func SetupIPTables(targetIPs []string, proxyPort int) error {
    if err := setupIPSet(targetIPs); err != nil {
        return fmt.Errorf("failed to setup ipset: %w", err)
    }

    if err := setupIPTablesRule(proxyPort); err != nil {
        return fmt.Errorf("failed to setup iptables: %w", err)
    }

    return nil
}

func setupIPSet(targetIPs []string) error {
    _, err := exec.Command("ipset", "list", ipsetName).CombinedOutput()
    if err != nil {
        if err := exec.Command("ipset", "create", ipsetName, "hash:ip").Run(); err != nil {
            return fmt.Errorf("failed to create ipset: %w", err)
        }
    } else {
        if err := exec.Command("ipset", "flush", ipsetName).Run(); err != nil {
            return fmt.Errorf("failed to flush ipset: %w", err)
        }
    }

    for _, ip := range targetIPs {
        cmd := exec.Command("ipset", "add", ipsetName, ip, "-exist")
        if err := cmd.Run(); err != nil {
            return fmt.Errorf("failed to add IP %s to ipset: %w", ip, err)
        }
    }

    return nil
}

func setupIPTablesRule(proxyPort int) error {
    portStr := fmt.Sprintf("%d", proxyPort)

    checkCmd := exec.Command("iptables", "-t", "nat", "-C", "PREROUTING",
        "-p", "tcp",
        "-m", "set", "--match-set", ipsetName, "dst",
        "-j", "REDIRECT", "--to-port", portStr)

    if err := checkCmd.Run(); err != nil {
        addCmd := exec.Command("iptables", "-t", "nat", "-A", "PREROUTING",
            "-p", "tcp",
            "-m", "set", "--match-set", ipsetName, "dst",
            "-j", "REDIRECT", "--to-port", portStr)

        output, err := addCmd.CombinedOutput()
        if err != nil {
            return fmt.Errorf("failed to add iptables rule: %w\nOutput: %s", err, string(output))
        }
    }

    return nil
}

func CleanupIPTables(proxyPort int) {
    portStr := fmt.Sprintf("%d", proxyPort)

    exec.Command("iptables", "-t", "nat", "-D", "PREROUTING",
        "-p", "tcp",
        "-m", "set", "--match-set", ipsetName, "dst",
        "-j", "REDIRECT", "--to-port", portStr).Run()

    exec.Command("ipset", "destroy", ipsetName).Run()
}

func VerifyIPTablesSetup() error {
    if _, err := exec.LookPath("iptables"); err != nil {
        return fmt.Errorf("iptables not found: %w", err)
    }

    if _, err := exec.LookPath("ipset"); err != nil {
        return fmt.Errorf("ipset not found: %w", err)
    }

    testCmd := exec.Command("iptables", "-t", "nat", "-L", "-n")
    if err := testCmd.Run(); err != nil {
        return fmt.Errorf("cannot access iptables (are you running as root?): %w", err)
    }

    return nil
}