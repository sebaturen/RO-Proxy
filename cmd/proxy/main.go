package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"roproxy/internal/config"
	"roproxy/internal/ipc"
	"roproxy/internal/proxy"
)

const SocketPath = "./roproxy.sock"

func main() {
	fmt.Println("RO-Proxy - Proxy Process (Hot-Swap Safe)")
	fmt.Println("=========================================")

	// Handle signals
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	go func() {
		sig := <-sigChan
		fmt.Printf("\n[Proxy] Signal received: %v - Shutting down...\n", sig)
		cancel()
		<-sigChan
		fmt.Println("[Proxy] Force exit...")
		os.Exit(1)
	}()

	// Load config
	cfg, err := config.Load("config.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[Proxy] Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize IPC client (connects to Analyzer)
	ipcClient := ipc.NewClient(SocketPath)
	defer ipcClient.Close()
	fmt.Printf("[Proxy] IPC client initialized (socket: %s)\n", SocketPath)

	// Store IPC client globally for connections to use
	proxy.SetIPCClient(ipcClient)

	// Initialize IPTables
	proxy.InitializeIPTables(cfg.TargetIPs, cfg.ListenPort)
	defer proxy.CleanupIPTables(cfg.ListenPort)
	fmt.Printf("[Proxy] IPTables configured for port %d\n", cfg.ListenPort)

	// Start proxy (includes SetProxy and SetRecording)
	p := proxy.New(cfg)
	proxy.Start(p, ctx)
	
	// Start recording file watcher (allows Analyzer to toggle recording via flag file)
	stopWatcher := make(chan struct{})
	go proxy.StartRecordingFileWatcher(stopWatcher)
	defer close(stopWatcher)
	
	fmt.Printf("[Proxy] Listening on port %d...\n", cfg.ListenPort)
	fmt.Println("[Proxy] Press Ctrl+C to stop (WARNING: This will disconnect all game clients!)")
	fmt.Println()

	// Wait for shutdown
	<-ctx.Done()
	fmt.Println("[Proxy] Shutdown complete")
}
