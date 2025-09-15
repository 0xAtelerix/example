package main

import (
	"context"
	"errors"
	"flag"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/txpool"
	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
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
	UseFiber           bool
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

const shutdownGrace = 10 * time.Second

func Run(ctx context.Context, args RuntimeArgs, ready chan<- int) {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Cancel on SIGINT/SIGTERM too (centralized; no per-runner signal goroutines needed)
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

	defer localDB.Close()

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

	defer appchainDB.Close()

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

	if args.UseFiber {
		runFiber(ctx, args, txPool, runErr, ready)

		return
	}
	// else: keep your net/http path if you want both
	runStdHTTP(ctx, args, txPool, runErr, ready)
}

func runStdHTTP(
	ctx context.Context,
	args RuntimeArgs,
	txPool *txpool.TxPool[application.Transaction[application.Receipt], application.Receipt],
	runErr chan error,
	ready chan<- int,
) {
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

	select {
	case <-ctx.Done():
		log.Info().Str("shutting down", ctx.Err().Error()).Msg("Received shutdown signal")

		//nolint:contextcheck // shutdown, the context above is already expired
		if err := server.Shutdown(context.Background()); err != nil {
			log.Error().Err(err).Msg("Failed to shutdown JSON-RPC server gracefully")

			_ = server.Close()
		}

		<-serveErr // drain

	case err := <-serveErr:
		// Server exited on its own
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("HTTP server crashed")
		}

	case err := <-runErr:
		if err != nil {
			log.Error().Err(err).Msg("Appchain stopped with error")
		}

		//nolint:contextcheck // shutdown, the context above is already expired
		if err := server.Shutdown(context.Background()); err != nil {
			log.Error().Err(err).Msg("Failed to shutdown JSON-RPC server gracefully")
		}

		<-serveErr
	}
}

func runFiber(
	ctx context.Context,
	args RuntimeArgs,
	txPool *txpool.TxPool[application.Transaction[application.Receipt], application.Receipt],
	runErr chan error,
	ready chan<- int,
) {
	// Use faster JSON codec (optional but recommended).
	app := fiber.New(fiber.Config{
		ReadTimeout: 0, // we already limit at TCP layer/timeouts around listener
		JSONEncoder: json.Marshal,
		JSONDecoder: json.Unmarshal,
		// You can tune these if needed:
		// Prefork:           true, // beware on macOS; better for Linux prod with SO_REUSEPORT
		// ReduceMemoryUsage: true,
	})

	// Reuse your existing net/http handler without rewriting it.
	h := &api.RPCServer{Pool: txPool}
	app.All("/rpc", adaptor.HTTPHandler(h))

	// Context-aware listener (satisfies `noctx` linter).
	lc := &net.ListenConfig{}

	ln, err := lc.Listen(ctx, "tcp", args.RPCPort) // ":0" allowed
	if err != nil {
		log.Fatal().Err(err).Msg("listen rpc")
	}

	// Publish chosen port (for your benchmark that starts on :0).
	if ready != nil {
		if ta, ok := ln.Addr().(*net.TCPAddr); ok {
			ready <- ta.Port
		} else {
			ready <- 0
		}

		close(ready)
	}

	// Serve in background; Fiber provides Listener().
	serveErr := make(chan error, 1)

	go func() { serveErr <- app.Listener(ln) }()

	select {
	case <-ctx.Done():
		// Start graceful shutdown.
		shutdownCh := make(chan struct{})

		go func() {
			_ = app.Shutdown()

			close(shutdownCh)
		}()

		select {
		case <-shutdownCh:
		case <-time.After(shutdownGrace):
			// Force-close listener if needed (rare).
			_ = ln.Close()
		}

		// Drain serveErr (Fiber returns error on closed listener).
		<-serveErr

	case err := <-serveErr:
		// Crashed on its own.
		if err != nil && !errors.Is(err, net.ErrClosed) {
			log.Fatal().Err(err).Msg("Fiber server crashed")
		}

	case err := <-runErr:
		log.Error().Err(err).Msg("Appchain stopped with error")
	}
}
