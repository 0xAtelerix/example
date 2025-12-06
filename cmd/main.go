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
	"github.com/0xAtelerix/sdk/gosdk/txpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/0xAtelerix/example/application"
	"github.com/0xAtelerix/example/application/api"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Parse command line flags
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	configPath := fs.String("config", "", "Path to config.yaml (optional)")
	_ = fs.Parse(os.Args[1:])

	// Load config from file or use defaults
	var (
		cfg *Config
		err error
	)

	if *configPath != "" {
		cfg, err = LoadConfig(*configPath)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to load config")
		}

		log.Info().Str("config", *configPath).Msg("Loaded config")
	} else {
		cfg = DefaultConfig()

		log.Info().Msg("Using default config")
	}

	// Setup logging
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).
		Level(zerolog.Level(cfg.LogLevel))
	ctx = log.With().Logger().WithContext(ctx)

	// Handle shutdown signals
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := Run(ctx, cfg); err != nil {
		log.Fatal().Err(err).Msg("Failed to run appchain")
	}
}

// Run starts the appchain with the given config. Exported for testing.
func Run(ctx context.Context, cfg *Config) error {
	// Initialize all common components (SDK Defaults if not provided)
	appInit, err := gosdk.Init(ctx, gosdk.InitConfig{
		ChainID:      cfg.ChainID,
		DataDir:      cfg.DataDir,
		EmitterPort:  cfg.EmitterPort,
		CustomTables: application.Tables(),
		Logger:       &log.Logger,
	})
	if err != nil {
		return fmt.Errorf("init appchain: %w", err)
	}
	defer appInit.Close()

	// Create transaction pool
	txPool := txpool.NewTxPool[application.Transaction[application.Receipt]](appInit.LocalDB)

	// Create appchain with app-specific logic
	appchain := gosdk.NewAppchain(
		appInit,
		gosdk.NewDefaultBatchProcessor[application.Transaction[application.Receipt]](
			application.NewExtBlockProcessor(appInit.Multichain),
			appInit.Multichain,
			appInit.Subscriber,
		),
		application.BlockConstructor,
		txPool,
	)

	// Initialize dev validator set (local development only)
	if err := gosdk.InitDevValidatorSet(ctx, appInit.AppchainDB); err != nil {
		return fmt.Errorf("init dev validator set: %w", err)
	}

	// Initialize app-specific genesis state
	if err := application.InitializeGenesis(ctx, appInit.AppchainDB); err != nil {
		return fmt.Errorf("init genesis state: %w", err)
	}

	// Run appchain in background
	go func() {
		if err := appchain.Run(ctx); err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("Appchain error")
		}
	}()

	// Setup and start JSON-RPC server
	rpcServer := rpc.NewStandardRPCServer(nil)
	rpcServer.AddMiddleware(api.NewExampleMiddleware(log.Logger))

	rpc.AddStandardMethods[
		application.Transaction[application.Receipt],
		application.Receipt,
		application.Block,
	](rpcServer, appInit.AppchainDB, txPool, cfg.ChainID)

	api.NewCustomRPC(rpcServer, appInit.AppchainDB).AddRPCMethods()

	return rpcServer.StartHTTPServer(ctx, cfg.RPCPort)
}
