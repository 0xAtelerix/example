package main

import (
	"context"
	"encoding/binary"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/0xAtelerix/sdk/gosdk/rpc"
	"github.com/0xAtelerix/sdk/gosdk/txpool"
	"github.com/fxamacker/cbor/v2"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	mdbxlog "github.com/ledgerwatch/log/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/0xAtelerix/example/api"
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
	config := gosdk.MakeAppchainConfig(ChainID, map[apptypes.ChainType]string{})

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

func Run(ctx context.Context, args RuntimeArgs, _ chan<- int) {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Cancel on SIGINT/SIGTERM too (centralized; no per-runner signal goroutines needed)
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	config := gosdk.MakeAppchainConfig(ChainID, map[apptypes.ChainType]string{})

	extChainDBs, err := gosdk.NewMultichainStateAccessDB(config.MultichainStateDB)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to open external chain dbs")
	}

	multiState := gosdk.NewMultichainStateAccess(extChainDBs)

	config.EmitterPort = args.EmitterPort
	config.AppchainDBPath = args.AppchainDBPath
	config.EventStreamDir = args.EventStreamDir
	config.TxStreamDir = args.TxStreamDir

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

	subscriber, err := gosdk.NewSubscriber(ctx, appchainDB)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create event subscriber")
	}

	stateTransition := gosdk.NewBatchProcesser[application.Transaction[application.Receipt]](
		application.NewStateTransition(),
		multiState,
		subscriber,
	)

	log.Info().Msg("Starting appchain...")

	// Add genesis validator set, Temp Solution
	if err := appchainDB.Update(ctx, func(txn kv.RwTx) error {
		valsetMap := map[gosdk.ValidatorID]gosdk.Stake{
			1: 10,
			2: 20,
			3: 30,
		}

		valset := gosdk.NewValidatorSet(valsetMap)

		valsetBytes, err := cbor.Marshal(valset)
		if err != nil {
			return err
		}

		var key [4]byte
		binary.BigEndian.PutUint32(key[:], 0)

		return txn.Put(gosdk.ValsetBucket, key[:], valsetBytes)
	}); err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize genesis validator set")
	}

	appchainExample := gosdk.NewAppchain(
		stateTransition,
		application.BlockConstructor,
		txPool,
		config,
		appchainDB,
		subscriber,
		multiState,
		txBatchDB)

	// Initialize genesis accounts and trading pairs after all databases are ready
	log.Info().Msg("Initializing genesis state...")

	if err := application.InitializeGenesis(ctx, appchainDB); err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize genesis state")
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

	// Add custom RPC methods
	api.NewCustomRPC(rpcServer, appchainDB).AddRPCMethods()

	log.Info().Msg("Starting RPC server on :" + args.RPCPort)

	if err := rpcServer.StartHTTPServer(ctx, args.RPCPort); err != nil {
		log.Fatal().Err(err).Msg("Failed to start RPC server")
	}
}
