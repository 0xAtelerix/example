package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/rpc"
	"github.com/0xAtelerix/sdk/gosdk/txpool"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	mdbxlog "github.com/ledgerwatch/log/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/0xAtelerix/example/application"
)

const ChainID = 42

type RuntimeArgs struct {
	EmitterPort    string
	AppchainDBPath string
	EventStreamDir string
	TxStreamDir    string
	LocalDBPath    string
	RPCPort        string
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
	rpcPort := fs.String("rpc-port", ":8080", "Port for the JSON-RPC server")

	_ = fs.Parse(os.Args[1:])

	args := RuntimeArgs{
		EmitterPort:    *emitterPort,
		AppchainDBPath: *appchainDBPath,
		EventStreamDir: *streamDir,
		TxStreamDir:    *txDir,
		LocalDBPath:    *localDBPath,
		RPCPort:        *rpcPort,
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
		}).Open()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to appchain mdbx database")
	}

	defer appchainDB.Close()

	txPool := txpool.NewTxPool[application.Transaction[application.Receipt]](
		localDB,
	)

	txBatchDB, err := mdbx.NewMDBX(mdbxlog.New()).
		Path(config.TxStreamDir).
		WithTableCfg(func(_ kv.TableCfg) kv.TableCfg {
			return gosdk.TxBucketsTables()
		}).
		Readonly().Open()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to tx batch mdbx database")
	}

	log.Info().Msg("Starting appchain...")

	appchainExample, err := gosdk.NewAppchain(
		stateTransition,
		application.BlockConstructor,
		txPool,
		config,
		appchainDB,
		txBatchDB)
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

	rpcServer := rpc.NewStandardRPCServer()
	rpc.AddStandardMethods(rpcServer, appchainDB, txPool)
	log.Info().Msg("Starting RPC server on :" + args.RPCPort)
	if err := rpcServer.StartHTTPServer(ctx, args.RPCPort); err != nil {
		log.Fatal().Err(err).Msg("Failed to start RPC server")
	}
}
