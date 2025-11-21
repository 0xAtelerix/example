VERSION=v2.4.0
ASC=npx --prefix as asc
WASM_TESTDATA_DIR=wasmstrategy/testdata
WASM_TESTDATA=$(WASM_TESTDATA_DIR)/uniswap_strategy.wasm \
	$(WASM_TESTDATA_DIR)/forbidden_import.wasm \
	$(WASM_TESTDATA_DIR)/memory_import.wasm

run:
	go run cmd/main.go \
                                        -emitter-port=:50051 \
                                        -db-path=/tmp/example/db/ \
                                        -tx-dir=/tmp/consensus/fetcher/snapshots/42 \
                                        -local-db-path=/tmp/example/test_tmp \
                                        -stream-dir=/tmp/consensus/events/ \
                                        -multichain-config=./debug/multichain.json \
                                        -rpc-port=:8080

dockerbuild:
	DOCKER_BUILDKIT=1 docker build --ssh default -t appchain:latest .

up:
	@echo "🔼 Starting containers..."
	docker compose up -d

build:
	DOCKER_BUILDKIT=1 docker compose build --ssh default

down:
	docker compose down

restart: down up

clean:
	rm -Rdf appchain multichain test_consensus_app test_consensus app_data pelacli_data


tidy:
	go mod tidy

wasm-deps:
	npm --prefix as ci

wasm-build: wasm-deps
	npm --prefix as run build

wasm-testdata: wasm-deps $(WASM_TESTDATA)

$(WASM_TESTDATA_DIR)/uniswap_strategy.wasm: $(WASM_TESTDATA_DIR)/as/uniswap_strategy.ts
	$(ASC) $< --runtime stub --optimize --shrinkLevel 2 --noAssert -o $@

$(WASM_TESTDATA_DIR)/forbidden_import.wasm: $(WASM_TESTDATA_DIR)/as/forbidden_import.ts
	$(ASC) $< --runtime stub --optimize --shrinkLevel 2 --noAssert -o $@

$(WASM_TESTDATA_DIR)/memory_import.wasm: $(WASM_TESTDATA_DIR)/as/memory_import.ts
	$(ASC) $< --runtime stub --optimize --shrinkLevel 2 --noAssert --importMemory -o $@

tests:
	go test -short -timeout 20m -failfast -shuffle=on -v ./... $(params)

race-tests:
	go test -race -short -timeout 30m -failfast -shuffle=on -v ./... $(params)



lints-docker: # 'sed' matches version in this string 'golangci-lint@xx.yy.zzz'
	echo "⚙️ Used lints version: " $(VERSION)
	docker run --rm -v $$(pwd):/app -w /app golangci/golangci-lint:$(VERSION) golangci-lint run -v  --timeout 10m

deps-local:
	go mod download
	go install github.com/bufbuild/buf/cmd/buf@latest
	go get google.golang.org/grpc@v1.75.0
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $$(go env GOPATH)/bin $(VERSION)

deps-ci:
	go mod download
	go install github.com/bufbuild/buf/cmd/buf@latest
	go get google.golang.org/grpc@v1.75.0

lints:
	$$(go env GOPATH)/bin/golangci-lint run ./... -v --timeout 10m

lints-fix:
	$$(go env GOPATH)/bin/golangci-lint run ./... -v --timeout 10m --fix
