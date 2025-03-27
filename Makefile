
run:
	GOPRIVATE=github.com/0xAtelerix/* go run cmd/main.go \
                                        -chain-id=42 \
                                        -emitter-port=:50051 \
                                        -db-path=./test \
                                        -tmp-db-path=./test_tmp \
                                        -stream-dir=/tmp/atxsnapshot/snapshot.data \
                                        -rpc-port=:8080
get:
	GOPRIVATE=github.com/0xAtelerix/* go mod download github.com/0xAtelerix/sdk/gosdk

deps:
	GOPRIVATE=github.com/0xAtelerix/* go mod download

tidy:
	GOPRIVATE=github.com/0xAtelerix/* go mod tidy

env:
	go env -w GOPRIVATE=github.com/0xAtelerix/sdkenv:
	go env -w GOPRIVATE=github.com/0xAtelerix/sdk
clean:
	rm -r ./test/*
	rm -r ./test_tmp/*

