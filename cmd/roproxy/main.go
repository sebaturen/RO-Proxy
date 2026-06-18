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

    "roproxy/internal/config"
    "roproxy/internal/proxy"
)

var verbose bool

func main() {
    flag.BoolVar(&verbose, "logs", false, "Enable verbose logging")
    flag.Parse()

    if !verbose {
        log.SetOutput(io.Discard)
    } else {
        log.SetFlags(log.LstdFlags | log.Lshortfile)
    }

    fmt.Println("ROProxy - Transparent TCP Proxy")

    cfg, err := config.Load("config.json")
    if err != nil {
        fmt.Printf("Failed to load config: %v\n", err)
        os.Exit(1)
    }

    fmt.Printf("Configuration loaded: listen_port=%d, allowed_ips=%d\n",
        cfg.ListenPort, len(cfg.TargetIPs))

    if verbose {
        fmt.Println("Verbose logging enabled")
    }

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    p := proxy.New(cfg)

    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

    var wg sync.WaitGroup
    wg.Add(1)
    go func() {
        defer wg.Done()
        if err := p.Start(ctx); err != nil {
            fmt.Printf("Proxy failed: %v\n", err)
            os.Exit(1)
        }
    }()

    <-sigChan
    fmt.Println("Shutdown signal received, stopping proxy...")
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
        fmt.Println("Proxy stopped gracefully")
    case <-shutdownCtx.Done():
        fmt.Println("Shutdown timeout exceeded, forcing exit")
    }
}
