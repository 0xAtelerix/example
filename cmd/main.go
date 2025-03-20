package main

import (
	"context"
	"github.com/0xAtelerix/example"
	"log"
	"net/http"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/txpool"
)

func main() {
	//input to appchain
	config := gosdk.MakeAppchainConfig(42)

	stateTransition := gosdk.BatchProcesser[*example.ExampleTransaction]{
		example.NewStateTransitionExample[*example.ExampleTransaction](),
	}
	rootCalculator := example.NewRootCalculatorExample()
	txPool, err := txpool.NewTxPool[*example.ExampleTransaction](config.TmpDBPath)
	if err != nil {
		log.Fatal(err)
	}

	appchainExample, err := gosdk.NewAppchain(
		stateTransition,
		rootCalculator,
		example.AppchainExampleBlockConstructor,
		txPool,
		config)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		http.Handle("/rpc", &example.RPCServer{Pool: txPool})

		// Запуск сервера
		log.Println("JSON-RPC сервер запущен на :8080")
		log.Fatal(http.ListenAndServe(":8080", nil))
	}()

	//init your tmp db if you need it. It won't be a part of consensus
	//here you could write your own api, using db from appchainExample.AppchainDB
	err = appchainExample.Run(context.TODO())
	if err != nil {
		log.Fatal(err)
	}
}
