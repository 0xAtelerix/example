
run:
	GOPRIVATE=github.com/0xAtelerix/* go run cmd/main.go \
                                        -emitter-port=:50051 \
                                        -db-path=./test \
                                        -local-db-path=./test_tmp \
                                        -stream-dir=./test_consenus/snapshot.data \
                                        -rpc-port=:8080
get:
	GOPRIVATE=github.com/0xAtelerix/* go mod download github.com/0xAtelerix/sdk/gosdk

env:
	go env -w GOPRIVATE=github.com/0xAtelerix/sdkenv:
	go env -w GOPRIVATE=github.com/0xAtelerix/sdk
	go env -w GOPRIVATE=github.com/0xAtelerix/*

dockerrun:
	docker run --rm \
	  -v $(PWD)/test_consenus:/test_consenus \
	  b00ris/consensusnode:latest \
	  --snapshot-dir=/test_consenus \
	  --appchain=1=host.docker.internal:50051

dockerbuild:
	DOCKER_BUILDKIT=1 docker build --ssh default -t abc/appchain:latest .

## Полный запуск: сборка и запуск контейнеров
up: build
	@echo "🔼 Starting containers..."
	docker compose up

## Сборка с SSH-ключом
build:
	DOCKER_BUILDKIT=1 docker compose build --ssh default

## Остановить и удалить
down:
	docker compose down

## Перезапуск
restart: down up

clean:
	rm -Rdf appchain localdb test chaindb test_tmp test_consenus_app test_consenus/events test_consenus/fetcher

tidy:
	GOPRIVATE=github.com/0xAtelerix/* go mod tidy

tests:
	go test -short -timeout 20m -failfast -shuffle=on -v ./... $(params)

race-tests:
	go test -race -short -timeout 30m -failfast -shuffle=on -v ./... $(params)

VERSION=v2.4.0

lints-docker: # 'sed' matches version in this string 'golangci-lint@xx.yy.zzz'
	echo "⚙️ Used lints version: " $(VERSION)
	docker run --rm -v $$(pwd):/app -w /app golangci/golangci-lint:$(VERSION) golangci-lint run -v  --timeout 10m

deps-local:
	GOPRIVATE=github.com/0xAtelerix/* go mod download
	GOPRIVATE=github.com/0xAtelerix/* go install github.com/bufbuild/buf/cmd/buf@latest
	GOPRIVATE=github.com/0xAtelerix/* go get google.golang.org/grpc@v1.75.0
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $$(go env GOPATH)/bin $(VERSION)

deps-ci:
	GOPRIVATE=github.com/0xAtelerix/* go mod download
	GOPRIVATE=github.com/0xAtelerix/* go install github.com/bufbuild/buf/cmd/buf@latest
	GOPRIVATE=github.com/0xAtelerix/* go get google.golang.org/grpc@v1.75.0

lints:
	$$(go env GOPATH)/bin/golangci-lint run ./... -v --timeout 10m

.PHONY: coverage
coverage:
	go test -short -timeout 20m -failfast -shuffle=on -v ./... -coverprofile=cover.out -covermode=atomic -coverpkg=./... $(params)
	go tool cover -html=cover.out -o coverage.html
