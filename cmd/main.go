package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/rpc"
	"github.com/rs/zerolog/log"

	"github.com/0xAtelerix/example/application"
	"github.com/0xAtelerix/example/application/api"
	"github.com/0xAtelerix/example/application/metrics"
)

func main() {
	// Parse command line flags
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	configPath := fs.String("config", "", "Path to config.yaml (optional)")
	_ = fs.Parse(os.Args[1:])

	// Load config from file or use defaults
	cfg, err := application.LoadConfig(*configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}

	log.Info().Msg("Config loaded")

	// Setup logging
	ctx := gosdk.SetupLogger(context.Background(), cfg.LogLevel)

	// signal.NotifyContext provides cancellation on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := Run(ctx, cfg); err != nil {
		log.Fatal().Err(err).Msg("Failed to run appchain")
	}
}

// Run starts the appchain with the given config. Exported for testing.
func Run(ctx context.Context, cfg *application.AppConfig) error {
	// Add custom tables to config
	cfg.CustomTables = application.Tables()

	// Stage 1: Initialize storage and config (logger comes from context)
	appInit, err := gosdk.InitApp[application.Transaction](ctx, cfg.InitConfig)
	if err != nil {
		return fmt.Errorf("init storage: %w", err)
	}
	defer appInit.Close()

	// Subscribe to bridge contracts on external chains
	application.SubscribeBridgeContracts(appInit.Storage.Subscriber(), cfg)

	// Stage 2: Create appchain with batch processor
	app := gosdk.NewAppchain(
		appInit.Storage,
		appInit.Config,
		gosdk.NewDefaultBatchProcessor[application.Transaction](
			application.NewExtBlockProcessor(appInit.Storage.Multichain(), cfg),
			appInit.Storage.Multichain(),
			appInit.Storage.Subscriber(),
		),
		application.BlockConstructor,
	)

	// Initialize dev validator set (local development only)
	if err := gosdk.InitDevValidatorSet(ctx, appInit.Storage.AppchainDB()); err != nil {
		return fmt.Errorf("init dev validator set: %w", err)
	}

	// Setup JSON-RPC server
	rpcServer := rpc.NewStandardRPCServer(nil)

	// Add standard RPC methods for explorer compatibility
	rpc.AddStandardMethods[
		application.Transaction,
		application.Receipt,
		application.Block,
	](rpcServer, appInit.Storage.AppchainDB(), appInit.Storage.TxPool(), appInit.Config.ChainID)

	// Add custom bridge RPC methods
	api.NewCustomRPC(rpcServer, appInit.Storage.AppchainDB(), cfg).AddRPCMethods()

	// Error channel for goroutines
	errCh := make(chan error, 3)

	// Run appchain in background
	go func() {
		errCh <- app.Run(ctx)
	}()

	// Run RPC server in background
	go func() {
		errCh <- rpcServer.StartHTTPServer(ctx, appInit.Config.RPCPort)
	}()

	// Run metrics server if configured
	if cfg.MetricsPort > 0 {
		go func() {
			errCh <- metrics.StartServer(ctx, cfg.MetricsPort)
		}()
	}

	// Wait for shutdown signal or error
	select {
	case <-ctx.Done():
		log.Ctx(ctx).Info().Msg("Shutdown signal received")

		return nil
	case err := <-errCh:
		log.Ctx(ctx).Error().Err(err).Msg("Appchain error")

		return err
	}
}
