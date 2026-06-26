package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    "roproxy/internal/common"
    "roproxy/internal/config"
    "roproxy/internal/proxy"
    "roproxy/internal/tui"
)

func main() {
    // Handle Ctrl+C with multiple signal types
    ctx, cancel := context.WithCancel(context.Background())
    sigChan := make(chan os.Signal, 2)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
    go func() {
        sig := <-sigChan
        fmt.Printf("Stop sign: %v. stop all...\n", sig)
        cancel()
        <-sigChan
		fmt.Println("[!] Forzando salida inmediata...")
		os.Exit(1)
    }()

    // Load config
    cfg, err := config.Load("config.json")
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
        os.Exit(1)
    }

    // =============
    // INIT SERVICES
    // =============

    // API
    common.InitAPIConsumer(cfg)
    // Logs
    common.InitializeGlobalLogQueue()
    // IPTables settings
    proxy.InitializeIPTables(cfg.TargetIPs, cfg.ListenPort)
    defer proxy.CleanupIPTables(cfg.ListenPort)
    // Dashboard
    dashboard := tui.NewDashboard()
    tui.StartUIConsumer(dashboard)
    defer dashboard.Stop()
    // Proxy
    p := proxy.New(cfg)
    proxy.Start(p, ctx)
    proxy.StartMonitoring(cfg)

    // Run dashboard (blocks until quit)
    if err := dashboard.Start(); err != nil {
        fmt.Fprintf(os.Stderr, "\n\nDashboard error: %v\n", err)
    }

    // Cleanup
    cancel()
}
