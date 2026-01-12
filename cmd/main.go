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
)

func main() {
	// Parse command line flags
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	configPath := fs.String("config", "", "Path to config.yaml (optional)")
	_ = fs.Parse(os.Args[1:])

	// Load config from file or use defaults
	var (
		cfg *gosdk.InitConfig
		err error
	)

	if *configPath != "" {
		cfg, err = gosdk.LoadConfig(*configPath)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to load config")
		}

		log.Info().Str("config", *configPath).Msg("Loaded config")
	} else {
		cfg = &gosdk.InitConfig{}

		log.Info().Msg("Using default config")
	}

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
func Run(ctx context.Context, cfg *gosdk.InitConfig) error {
	// Add custom tables to config
	cfg.CustomTables = application.Tables()

	// Stage 1: Initialize storage and config (logger comes from context)
	appInit, err := gosdk.InitApp[application.Transaction[application.Receipt]](ctx, *cfg)
	if err != nil {
		return fmt.Errorf("init storage: %w", err)
	}
	defer appInit.Close()

	// Stage 2: Create appchain with batch processor
	app := gosdk.NewAppchain(
		appInit.Storage,
		appInit.Config,
		gosdk.NewDefaultBatchProcessor[application.Transaction[application.Receipt]](
			application.NewExtBlockProcessor(appInit.Storage.Multichain()),
			appInit.Storage.Multichain(),
			appInit.Storage.Subscriber(),
		),
		application.BlockConstructor,
	)

	// Initialize dev validator set (local development only)
	if err := gosdk.InitDevValidatorSet(ctx, appInit.Storage.AppchainDB()); err != nil {
		return fmt.Errorf("init dev validator set: %w", err)
	}

	// Initialize app-specific genesis state
	if err := application.InitializeGenesis(ctx, appInit.Storage.AppchainDB()); err != nil {
		return fmt.Errorf("init genesis state: %w", err)
	}

	// Setup JSON-RPC server
	rpcServer := rpc.NewStandardRPCServer(nil)
	rpcServer.AddMiddleware(api.NewExampleMiddleware(log.Logger))

	rpc.AddStandardMethods[
		application.Transaction[application.Receipt],
		application.Receipt,
		application.Block,
	](rpcServer, appInit.Storage.AppchainDB(), appInit.Storage.TxPool(), appInit.Config.ChainID)

	api.NewCustomRPC(rpcServer, appInit.Storage.AppchainDB()).AddRPCMethods()

	// Error channel for goroutines
	errCh := make(chan error, 2)

	// Run appchain in background
	go func() {
		errCh <- app.Run(ctx)
	}()

	// Run RPC server in background
	go func() {
		errCh <- rpcServer.StartHTTPServer(ctx, appInit.Config.RPCPort)
	}()

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
