package main

import (
	"context"
	"flag"
	"github.com/0xAtelerix/example"
	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/txpool"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// CLI flags
	var (
		chainID        = flag.Uint64("chain-id", 42, "Chain ID of the appchain")
		emitterPort    = flag.String("emitter-port", ":50051", "Emitter gRPC port")
		appchainDBPath = flag.String("db-path", "./appchaindb", "Path to appchain DB")
		//todo rename to not a part of consensus
		tmpDBPath          = flag.String("tmp-db-path", "./tmpdb", "Path to temporary DB")
		streamDir          = flag.String("stream-dir", "", "Event stream directory")
		txDir              = flag.String("tx-dir", "", "Transaction stream directory")
		ethereumBlocksPath = flag.String("ethdb", "", "read only eth blocks db")
		solBlocksPath      = flag.String("soldb", "", "read only sol blocks db")
		rpcPort            = flag.String("rpc-port", ":8080", "Port for the JSON-RPC server")
	)

	_, _ = ethereumBlocksPath, solBlocksPath
	flag.Parse()

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	config := gosdk.MakeAppchainConfig(*chainID)
	config.EmitterPort = *emitterPort
	config.AppchainDBPath = *appchainDBPath
	config.TmpDBPath = *tmpDBPath
	config.EventStreamDir = *streamDir
	config.TxStreamDir = *txDir

	stateTransition := gosdk.BatchProcesser[*example.ExampleTransaction]{
		example.NewStateTransitionExample[*example.ExampleTransaction](),
	}
	rootCalculator := example.NewRootCalculatorExample()

	txPool, err := txpool.NewTxPool[*example.ExampleTransaction](config.TmpDBPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize transaction pool")
	}

	log.Info().Msg("Starting appchain...")
	appchainExample, err := gosdk.NewAppchain(
		stateTransition,
		rootCalculator,
		example.AppchainExampleBlockConstructor,
		txPool,
		config)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to start appchain")
	}

	// Context with cancel for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Catch system signals
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// Start JSON-RPC server in goroutine
	server := &http.Server{
		Addr: *rpcPort,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.DefaultServeMux.ServeHTTP(w, r)
		}),
	}
	http.Handle("/rpc", &example.RPCServer{Pool: txPool})

	go func() {
		log.Info().Str("rpc_port", *rpcPort).Msg("Starting JSON-RPC server")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("JSON-RPC server failed")
		}
	}()

	// Run appchain in goroutine
	runErr := make(chan error, 1)
	go func() {
		runErr <- appchainExample.Run(ctx)
	}()

	select {
	case sig := <-signalChan:
		log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
		cancel()
		if err := server.Shutdown(context.Background()); err != nil {
			log.Error().Err(err).Msg("Failed to shutdown JSON-RPC server gracefully")
		}
	case err := <-runErr:
		log.Error().Err(err).Msg("Appchain stopped with error")
		cancel()
		if err := server.Shutdown(context.Background()); err != nil {
			log.Error().Err(err).Msg("Failed to shutdown JSON-RPC server gracefully")
		}
	}
}
