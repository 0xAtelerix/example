package main

import (
	"context"
	"flag"
	"os"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/rs/zerolog"

	"github.com/0xAtelerix/example/application"
)

const ChainID = 42

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
	ethereumBlocksPath := fs.String("ethdb", "", "read only eth blocks db")
	solBlocksPath := fs.String("soldb", "", "read only sol blocks db")
	rpcPort := fs.String("rpc-port", ":8080", "Port for the JSON-RPC server")
	logLevel := fs.Int("log-level", int(zerolog.DebugLevel), "Logging level")

	if *logLevel > int(zerolog.Disabled) {
		*logLevel = int(zerolog.DebugLevel)
	} else if *logLevel < int(zerolog.TraceLevel) {
		*logLevel = int(zerolog.TraceLevel)
	}

	_ = fs.Parse(os.Args[1:])

	args := application.RuntimeArgs{
		EmitterPort:        *emitterPort,
		AppchainDBPath:     *appchainDBPath,
		EventStreamDir:     *streamDir,
		TxStreamDir:        *txDir,
		LocalDBPath:        *localDBPath,
		EthereumBlocksPath: *ethereumBlocksPath,
		SolBlocksPath:      *solBlocksPath,
		RPCPort:            *rpcPort,
		LogLevel:           zerolog.Level(*logLevel),
	}

	application.Run(ctx, args, ChainID, nil)
}
