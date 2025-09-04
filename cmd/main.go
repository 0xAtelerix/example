package main

import (
	"context"
	"flag"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/txpool"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	mdbxlog "github.com/ledgerwatch/log/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/0xAtelerix/example/application"
	"github.com/0xAtelerix/example/application/api"
)

const ChainID = 42

type RuntimeArgs struct {
	EmitterPort        string
	AppchainDBPath     string
	EventStreamDir     string
	TxStreamDir        string
	LocalDBPath        string
	EthereumBlocksPath string
	SolBlocksPath      string
	RPCPort            string
}

func main() {
	// Context with cancel for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	RunCLI(ctx)
}

func RunCLI(ctx context.Context) {
	config := gosdk.MakeAppchainConfig(ChainID)

	// Use a local FlagSet (no globals).
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	emitterPort := fs.String("emitter-port", config.EmitterPort, "Emitter gRPC port")
	appchainDBPath := fs.String("db-path", config.AppchainDBPath, "Path to appchain DB")
	streamDir := fs.String("stream-dir", config.EventStreamDir, "Event stream directory")
	txDir := fs.String("tx-dir", config.TxStreamDir, "Transaction stream directory")

	localDBPath := fs.String("local-db-path", "./localdb", "Path to local DB")
	ethereumBlocksPath := fs.String("ethdb", "", "read only eth blocks db")
	solBlocksPath := fs.String("soldb", "", "read only sol blocks db")
	rpcPort := fs.String("rpc-port", ":8080", "Port for the JSON-RPC server")

	_ = fs.Parse(os.Args[1:])

	args := RuntimeArgs{
		EmitterPort:        *emitterPort,
		AppchainDBPath:     *appchainDBPath,
		EventStreamDir:     *streamDir,
		TxStreamDir:        *txDir,
		LocalDBPath:        *localDBPath,
		EthereumBlocksPath: *ethereumBlocksPath,
		SolBlocksPath:      *solBlocksPath,
		RPCPort:            *rpcPort,
	}

	Run(ctx, args, nil)
}

func Run(ctx context.Context, args RuntimeArgs, ready chan<- int) {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	config := gosdk.MakeAppchainConfig(ChainID)

	config.EmitterPort = args.EmitterPort
	config.AppchainDBPath = args.AppchainDBPath
	config.EventStreamDir = args.EventStreamDir
	config.TxStreamDir = args.TxStreamDir

	stateTransition := gosdk.NewBatchProcesser[application.Transaction[application.Receipt]](
		application.NewStateTransition(),
	)

	localDB, err := mdbx.NewMDBX(mdbxlog.New()).
		Path(args.LocalDBPath).
		WithTableCfg(func(_ kv.TableCfg) kv.TableCfg {
			return txpool.Tables()
		}).
		Open()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to local mdbx database")
	}

	// инициализируем базу на нашей стороне
	appchainDB, err := mdbx.NewMDBX(mdbxlog.New()).
		Path(config.AppchainDBPath).
		WithTableCfg(func(_ kv.TableCfg) kv.TableCfg {
			return gosdk.MergeTables(
				gosdk.DefaultTables(),
				application.Tables(),
			)
		}).
		Open()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to appchain mdbx database")
	}

	txPool := txpool.NewTxPool[application.Transaction[application.Receipt]](
		localDB,
	)

	log.Info().Msg("Starting appchain...")

	appchainExample, err := gosdk.NewAppchain(
		stateTransition,
		application.BlockConstructor,
		txPool,
		config,
		appchainDB)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to start appchain")
	}

	// Catch system signals
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// Start JSON-RPC server in goroutine
	mux := http.NewServeMux()
	mux.Handle("/rpc", &api.RPCServer{Pool: txPool})

	lc := &net.ListenConfig{}

	ln, err := lc.Listen(ctx, "tcp", args.RPCPort) // ":0" allowed
	if err != nil {
		log.Fatal().Err(err).Msg("listen rpc")
	}

	if ready != nil {
		if ta, ok := ln.Addr().(*net.TCPAddr); ok {
			ready <- ta.Port // publish actual port
		} else {
			ready <- 0
		}

		close(ready)
	}

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// serve
	serveErr := make(chan error, 1)

	go func() { serveErr <- server.Serve(ln) }()

	// Run appchain in goroutine
	runErr := make(chan error, 1)

	go func() {
		select {
		case <-ctx.Done():
			// nothing to do
		case runErr <- appchainExample.Run(ctx, nil):
			// nothing to do
		}
	}()

	select {
	case <-ctx.Done():
		log.Info().Str("shutting down", ctx.Err().Error()).Msg("Received shutdown signal")

		//nolint:contextcheck // shutdown, the context above is already expired
		if err := server.Shutdown(context.Background()); err != nil {
			log.Error().Err(err).Msg("Failed to shutdown JSON-RPC server gracefully")
		}
	case sig := <-signalChan:
		log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")

		//nolint:contextcheck // shutdown, the context above is already expired
		if err := server.Shutdown(context.Background()); err != nil {
			log.Error().Err(err).Msg("Failed to shutdown JSON-RPC server gracefully")
		}
	case err := <-runErr:
		log.Error().Err(err).Msg("Appchain stopped with error")

		//nolint:contextcheck // shutdown, the context above is already expired
		if err := server.Shutdown(context.Background()); err != nil {
			log.Error().Err(err).Msg("Failed to shutdown JSON-RPC server gracefully")
		}
	}
}
