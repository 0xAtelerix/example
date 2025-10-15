package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/rpc"
	"github.com/0xAtelerix/sdk/gosdk/txpool"
	"github.com/fxamacker/cbor/v2"
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
	EmitterPort      string
	AppchainDBPath   string
	EventStreamDir   string
	TxStreamDir      string
	LocalDBPath      string
	RPCPort          string
	MutlichainConfig gosdk.MultichainConfig
	LogLevel         zerolog.Level
}

func main() {
	// Context with cancel for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	RunCLI(ctx)
}

func RunCLI(ctx context.Context) {
	config := gosdk.MakeAppchainConfig(ChainID, nil)

	// Use a local FlagSet (no globals).
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	emitterPort := fs.String("emitter-port", config.EmitterPort, "Emitter gRPC port")
	appchainDBPath := fs.String("db-path", config.AppchainDBPath, "Path to appchain DB")
	streamDir := fs.String("stream-dir", config.EventStreamDir, "Event stream directory")
	txDir := fs.String("tx-dir", config.TxStreamDir, "Transaction stream directory")

	localDBPath := fs.String("local-db-path", "./localdb", "Path to local DB")
	rpcPort := fs.String("rpc-port", ":8080", "Port for the JSON-RPC server")
	multichainConfigJSON := fs.String("multichain-config", "", "Multichain config JSON path")
	logLevel := fs.Int("log-level", int(zerolog.InfoLevel), "Logging level")

	if *logLevel > int(zerolog.Disabled) {
		*logLevel = int(zerolog.DebugLevel)
	} else if *logLevel < int(zerolog.TraceLevel) {
		*logLevel = int(zerolog.TraceLevel)
	}

	_ = fs.Parse(os.Args[1:])

	var mcDbs gosdk.MultichainConfig

	if multichainConfigJSON != nil && *multichainConfigJSON != "" {
		f, err := os.ReadFile(*multichainConfigJSON)
		if err != nil {
			log.Panic().Err(err).Msg("Error reading multichain config")
		}

		err = json.Unmarshal(f, &mcDbs)
		if err != nil {
			log.Warn().Err(err).Msg("Error unmarshalling multichain config")
		}
	}

	args := RuntimeArgs{
		EmitterPort:      *emitterPort,
		AppchainDBPath:   *appchainDBPath,
		EventStreamDir:   *streamDir,
		TxStreamDir:      *txDir,
		LocalDBPath:      *localDBPath,
		RPCPort:          *rpcPort,
		LogLevel:         zerolog.Level(*logLevel),
		MutlichainConfig: mcDbs,
	}

	Run(ctx, args, nil)
}

func Run(ctx context.Context, args RuntimeArgs, _ chan<- int) {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).Level(args.LogLevel)

	ctx = log.With().Logger().WithContext(ctx)

	// Cancel on SIGINT/SIGTERM too (centralized; no per-runner signal goroutines needed)
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	config := gosdk.MakeAppchainConfig(ChainID, args.MutlichainConfig)

	config.EmitterPort = args.EmitterPort
	config.AppchainDBPath = args.AppchainDBPath
	config.EventStreamDir = args.EventStreamDir
	config.TxStreamDir = args.TxStreamDir
	config.Logger = &log.Logger

	chainDBs, err := gosdk.NewMultichainStateAccessDB(args.MutlichainConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create multichain db")
	}

	msa := gosdk.NewMultichainStateAccess(chainDBs)

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

	subs, err := gosdk.NewSubscriber(ctx, appchainDB)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create subscriber")
	}

	stateTransition := gosdk.NewBatchProcesser[application.Transaction[application.Receipt]](
		application.NewStateTransition(msa),
		msa,
		subs,
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

	// fixme dynamic val set. Right now it is especially for local development with pelacli
	valset := &gosdk.ValidatorSet{Set: map[gosdk.ValidatorID]gosdk.Stake{0: 100}}

	var epochKey [4]byte
	binary.BigEndian.PutUint32(epochKey[:], 1)

	valsetData, err := cbor.Marshal(valset)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to marshal validator set data")
	}

	err = appchainDB.Update(ctx, func(tx kv.RwTx) error {
		return tx.Put(gosdk.ValsetBucket, epochKey[:], valsetData)
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to appchain mdbx database")
	}

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
		log.Fatal().Str("path", config.TxStreamDir).Err(err).Msg("Failed to tx batch mdbx database")
	}

	log.Info().Msg("Starting appchain...")

	appchainExample := gosdk.NewAppchain(
		stateTransition,
		application.BlockConstructor,
		txPool,
		config,
		appchainDB,
		subs,
		msa,
		txBatchDB,
	)

	if err != nil {
		log.Fatal().Err(err).Msg("Failed to start appchain")
	}

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

	rpcServer := rpc.NewStandardRPCServer(nil)

	// Optional: add middleware for logging
	rpcServer.AddMiddleware(api.NewExampleMiddleware(log.Logger))

	// Add standard RPC methods - Refer RPC readme in sdk for details
	rpc.AddStandardMethods(rpcServer, appchainDB, txPool)

	// Add custom RPC methods - Optional
	api.NewCustomRPC(rpcServer, appchainDB).AddRPCMethods()

	if err := rpcServer.StartHTTPServer(ctx, args.RPCPort); err != nil {
		log.Fatal().Err(err).Msg("Failed to start RPC server")
	}
}
