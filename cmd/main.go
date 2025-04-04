package main

import (
	"context"
	"flag"
	"github.com/0xAtelerix/example"
	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/txpool"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	mdbxlog "github.com/ledgerwatch/log/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

const ChainID = 42

func main() {
	// CLI flags

	config := gosdk.MakeAppchainConfig(ChainID)
	var (
		emitterPort    = flag.String("emitter-port", config.EmitterPort, "Emitter gRPC port")
		appchainDBPath = flag.String("db-path", config.AppchainDBPath, "Path to appchain DB")
		streamDir      = flag.String("stream-dir", config.EventStreamDir, "Event stream directory")
		txDir          = flag.String("tx-dir", config.TxStreamDir, "Transaction stream directory")

		localDBPath        = flag.String("local-db-path", "./localdb", "Path to local DB")
		ethereumBlocksPath = flag.String("ethdb", "", "read only eth blocks db")
		solBlocksPath      = flag.String("soldb", "", "read only sol blocks db")
		rpcPort            = flag.String("rpc-port", ":8080", "Port for the JSON-RPC server")
	)

	_, _ = ethereumBlocksPath, solBlocksPath
	flag.Parse()

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	config.EmitterPort = *emitterPort
	config.AppchainDBPath = *appchainDBPath
	config.EventStreamDir = *streamDir
	config.TxStreamDir = *txDir

	stateTransition := gosdk.BatchProcesser[*example.ExampleTransaction]{
		example.NewStateTransitionExample[*example.ExampleTransaction](),
	}

	localDB, err := mdbx.NewMDBX(mdbxlog.New()).
		Path(*localDBPath).
		WithTableCfg(func(defaultBuckets kv.TableCfg) kv.TableCfg {
			return txpool.TxPoolTables
		}).
		Open()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to local mdbx database")
	}
	txPool := txpool.NewTxPool[*example.ExampleTransaction](localDB)

	log.Info().Msg("Starting appchain...")
	appchainExample, err := gosdk.NewAppchain(
		stateTransition,
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
