package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"roproxy/internal/analyzer"
	"roproxy/internal/common"
	"roproxy/internal/config"
	"roproxy/internal/ipc"
	"roproxy/internal/tui"
)

const SocketPath = "./roproxy.sock"

func main() {
	fmt.Println("RO-Proxy - Analyzer Process (Hot-Swappable)")
	fmt.Println("============================================")

	// Handle signals
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	go func() {
		sig := <-sigChan
		fmt.Printf("\n[Analyzer] Signal received: %v - Shutting down...\n", sig)
		cancel()
		<-sigChan
		fmt.Println("[Analyzer] Force exit...")
		os.Exit(1)
	}()

	// Load config
	cfg, err := config.Load("config.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[Analyzer] Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize services
	common.InitAPIConsumer(cfg)
	common.InitializeGlobalLogQueue()
	common.StartMonitoring(cfg)
	fmt.Println("[Analyzer] Services initialized")

	// Start IPC server
	ipcServer, err := ipc.NewServer(SocketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[Analyzer] Failed to create IPC server: %v\n", err)
		os.Exit(1)
	}
	defer ipcServer.Close()
	ipcServer.Start(ctx)
	fmt.Printf("[Analyzer] IPC server listening on %s\n", SocketPath)

	// Start processor (receives frames and manages workers)
	processor := analyzer.NewProcessor(ipcServer.Frames(), cfg)
	processor.Start(ctx)
	defer processor.Stop()
	fmt.Println("[Analyzer] Packet processor started")

	// Start TUI dashboard
	dashboard := tui.NewDashboard(processor)
	tui.StartUIConsumer(dashboard)
	defer dashboard.Stop()

	fmt.Println("[Analyzer] Starting TUI dashboard...")
	fmt.Println("[Analyzer] This process is HOT-SWAPPABLE - restart anytime without disconnecting game clients")
	fmt.Println()

	// Run dashboard (blocks until quit)
	if err := dashboard.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "[Analyzer] Dashboard error: %v\n", err)
	}

	// Cleanup
	cancel()
	fmt.Println("[Analyzer] Shutdown complete")
}
