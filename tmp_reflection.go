package main

import (
	"fmt"
	"reflect"

	"github.com/0xAtelerix/sdk/gosdk/rpc"
)

func main() {
	srv := rpc.NewStandardRPCServer(nil)
	m := reflect.ValueOf(srv).MethodByName("handleRPC")
	fmt.Println(m.IsValid())
}
