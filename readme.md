## Use local gosdk version
Clone `example` and `sdk`, in the parent folder `0xatelerix` add `go.work`:

```
go 1.25.0

use (
	./example
	./sdk
)
```

Any `go` command can be used with and without go workspaces by adding `GOWORK=off` env variable.

## Useful requests
```
curl -s http://localhost:8080/rpc -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"SendTransaction","params":{"transaction":{"sender":"alice","value":42, "hash":"deadbeef"}}}'

curl -s http://localhost:8080/rpc -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":2,"method":"GetTransactionByHash","params":{"hash":"deadbeef"}}'

curl -s http://localhost:8080/rpc -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":2,"method":"GetTransactionStatus","params":{"hash":"deadbeef"}}'
```