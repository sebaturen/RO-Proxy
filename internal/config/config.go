package config

import (
    "encoding/json"
    "fmt"
    "net"
    "os"
)

type Config struct {
    ListenPort int      `json:"listen_port"`
    TargetIPs  []string `json:"target_ips"`
}

func Load(path string) (*Config, error) {
    file, err := os.Open(path)
    if err != nil {
        return nil, fmt.Errorf("cannot open config file %s: %w", path, err)
    }
    defer file.Close()

    var cfg Config
    decoder := json.NewDecoder(file)
    decoder.DisallowUnknownFields()

    if err := decoder.Decode(&cfg); err != nil {
        return nil, fmt.Errorf("cannot parse config: %w", err)
    }

    if err := cfg.validate(); err != nil {
        return nil, fmt.Errorf("invalid config: %w", err)
    }

    return &cfg, nil
}

func (c *Config) validate() error {
    if c.ListenPort <= 0 || c.ListenPort > 65535 {
        return fmt.Errorf("listen_port must be between 1 and 65535, got %d", c.ListenPort)
    }

    if len(c.TargetIPs) == 0 {
        return fmt.Errorf("target_ips cannot be empty")
    }

    for i, ip := range c.TargetIPs {
        if net.ParseIP(ip) == nil {
            return fmt.Errorf("target_ips[%d]: invalid IP address: %s", i, ip)
        }
    }

    return nil
}
