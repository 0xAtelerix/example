
run:
	GOPRIVATE=github.com/0xAtelerix/* go run cmd/main.go \
                                        -chain-id=42 \
                                        -emitter-port=:50051 \
                                        -db-path=./test \
                                        -tmp-db-path=./test_tmp \
                                        -stream-dir=./test_consenus/snapshot.data \
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