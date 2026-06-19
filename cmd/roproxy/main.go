package main

import (
    "context"
    "flag"
    "fmt"
    "os"
    "os/signal"
    "sync"
    "syscall"
    "time"

    "roproxy/internal/common"
    "roproxy/internal/config"
    "roproxy/internal/proxy"
    "roproxy/internal/tui"
)

var (
    captureServer bool
    captureClient bool
)

func main() {
    flag.BoolVar(&captureServer, "capture-server", true, "Capture server->client packets")
    flag.BoolVar(&captureClient, "capture-client", true, "Capture client->server packets")
    flag.Parse()

    fmt.Println("ROProxy - Transparent TCP Proxy")

    cfg, err := config.Load("config.json")
    if err != nil {
        fmt.Printf("Failed to load config: %v\n", err)
        os.Exit(1)
    }

    fmt.Printf("Configuration loaded: listen_port=%d, allowed_ips=%d\n",
        cfg.ListenPort, len(cfg.TargetIPs))

    if cfg.API != nil && cfg.API.URL != "" && cfg.API.Key != "" {
        common.InitAPIConsumer(cfg.API.URL, cfg.API.Key, false)
        fmt.Printf("API consumer initialized: %s\n", cfg.API.URL)
    }

    if err := proxy.VerifyIPTablesSetup(); err != nil {
        fmt.Printf("iptables verification failed: %v\n", err)
        fmt.Println("Make sure you:")
        fmt.Println("  1. Run as root (sudo ./roproxy)")
        fmt.Println("  2. Have iptables and ipset installed")
        os.Exit(1)
    }

    fmt.Println("Configuring iptables rules...")
    if err := proxy.SetupIPTables(cfg.TargetIPs, cfg.ListenPort); err != nil {
        fmt.Printf("Failed to setup iptables: %v\n", err)
        os.Exit(1)
    }

    fmt.Println("iptables configured successfully")
    time.Sleep(1 * time.Second) // Give user time to read messages

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    defer proxy.CleanupIPTables(cfg.ListenPort)

    p := proxy.New(cfg, false, captureServer, captureClient)
    dashboard := tui.NewDashboard(p, captureServer, captureClient)
    p.SetPacketLogger(dashboard)

    var wg sync.WaitGroup

    // Start proxy in background
    wg.Add(1)
    go func() {
        defer wg.Done()
        if err := p.Start(ctx); err != nil {
            dashboard.Log("[red]Proxy failed: %v[-]", err)
        }
    }()

    // Handle signals
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    go func() {
        <-sigChan
        dashboard.Log("[yellow]Shutdown signal received[-]")
        dashboard.Stop()
    }()

    dashboard.Log("[green]ROProxy started successfully[-]")
    dashboard.Log("[yellow]Listening on port %d[-]", cfg.ListenPort)
    dashboard.Log("[gray]Use keyboard controls to interact (Q to quit)[-]")

    // Run dashboard (blocks until quit)
    if err := dashboard.Start(); err != nil {
        fmt.Printf("Dashboard error: %v\n", err)
    }

    // Cleanup
    cancel()

    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer shutdownCancel()

    done := make(chan struct{})
    go func() {
        wg.Wait()
        close(done)
    }()

    select {
    case <-done:
        fmt.Println("\nProxy stopped gracefully")
    case <-shutdownCtx.Done():
        fmt.Println("\nShutdown timeout exceeded, forcing exit")
    }
}
