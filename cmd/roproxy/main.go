package main

import (
    "context"
    "flag"
    "fmt"
    "io"
    "log"
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

    // Disable all standard logging immediately
    log.SetOutput(io.Discard)
	
    // Handle Ctrl+C with multiple signal types
    ctx, cancel := context.WithCancel(context.Background())
    sigChan := make(chan os.Signal, 2)
    // Listen for multiple signal types (SSH can send different signals)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	
    var dashboardPtr *tui.Dashboard
    var shutdownOnce sync.Once
    go func() {
        <-sigChan
        shutdownOnce.Do(func() {
            cancel()
            if dashboardPtr != nil {
                dashboardPtr.Stop()
            }
            // Force exit after 1 second
            time.AfterFunc(1*time.Second, func() {
                os.Exit(0)
            })
        })
    }()

    cfg, err := config.Load("config.json")
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
        os.Exit(1)
    }

    if cfg.API != nil && cfg.API.URL != "" && cfg.API.Key != "" {
        common.InitAPIConsumer(cfg.API.URL, cfg.API.Key, false)
    }

    if err := proxy.VerifyIPTablesSetup(); err != nil {
        fmt.Fprintf(os.Stderr, "iptables verification failed: %v\n", err)
        fmt.Fprintln(os.Stderr, "Make sure you:")
        fmt.Fprintln(os.Stderr, "  1. Run as root (sudo ./roproxy)")
        fmt.Fprintln(os.Stderr, "  2. Have iptables and ipset installed")
        os.Exit(1)
    }

    if err := proxy.SetupIPTables(cfg.TargetIPs, cfg.ListenPort); err != nil {
        fmt.Fprintf(os.Stderr, "Failed to setup iptables: %v\n", err)
        os.Exit(1)
    }

    defer proxy.CleanupIPTables(cfg.ListenPort)

    p := proxy.New(cfg, false, captureServer, captureClient)
    dashboard := tui.NewDashboard(p, captureServer, captureClient)
    dashboardPtr = dashboard  // Set pointer for signal handler
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

    // Log initial messages after UI starts
    go func() {
        time.Sleep(100 * time.Millisecond)
        dashboard.Log("[green]✓ ROProxy started[-]")
        dashboard.Log("[yellow]Listening on port %d[-]", cfg.ListenPort)
        dashboard.Log("[gray]Configuration: %d allowed IPs[-]", len(cfg.TargetIPs))
        if cfg.API != nil && cfg.API.URL != "" {
            dashboard.Log("[gray]API consumer active: %s[-]", cfg.API.URL)
        }
    }()

    // Run dashboard (blocks until quit)
    if err := dashboard.Start(); err != nil {
        log.SetOutput(os.Stderr)
        fmt.Fprintf(os.Stderr, "\n\nDashboard error: %v\n", err)
    }

    // Cleanup
    cancel()

    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer shutdownCancel()

    done := make(chan struct{})
    go func() {
        wg.Wait()
        close(done)
    }()

    select {
    case <-done:
    case <-shutdownCtx.Done():
    }
}
