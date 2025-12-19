VERSION=v2.4.0

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

# CI targets (uses docker-compose.ci.yml as override)
COMPOSE_CI := docker compose -f docker-compose.yml -f docker-compose.ci.yml

ci-up:
	@echo "🔼 Starting CI containers with latest pelacli..."
	$(COMPOSE_CI) pull pelacli
	$(COMPOSE_CI) up -d --build

ci-down:
	$(COMPOSE_CI) down

ci-clean:
	rm -Rdf appchain multichain test_consensus_app test_consensus app_data pelacli_data

ci-logs:
	$(COMPOSE_CI) logs

ci-wait-healthy:
	@echo "⏳ Waiting for services to be healthy..."
	@for i in $$(seq 1 60); do \
		if curl -sf http://localhost:8080/health > /dev/null 2>&1; then \
			echo "✅ Appchain is healthy"; \
			exit 0; \
		fi; \
		echo "Waiting for appchain... ($$i/60)"; \
		sleep 2; \
	done; \
	echo "❌ Timeout waiting for appchain"; \
	$(COMPOSE_CI) logs; \
	exit 1

ci-test-blocks:
	@echo "🧪 Testing block production..."
	./test_txns.sh

ci-integration: ci-clean ci-up ci-wait-healthy ci-test-blocks
	@echo "✅ CI integration test passed!"

ci-integration-cleanup: ci-integration
	$(MAKE) ci-down ci-clean
