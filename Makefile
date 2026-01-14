VERSION=v2.4.0

# Run appchain with default config (zero-config)
run:
	go run ./cmd/...

# Run appchain with custom config file
run-config:
	go run ./cmd/... -config=config.yaml

# Build appchain binary
build-bin:
	go build -o bin/appchain ./cmd/...

dockerbuild:
	DOCKER_BUILDKIT=1 docker build --ssh default -t appchain:latest .

up:
	@echo "🔼 Starting containers..."
	docker compose up -d

build:
	DOCKER_BUILDKIT=1 docker compose build --ssh default

down:
	docker compose down

logs:
	docker compose logs

restart: down up

clean:
	rm -Rdf data

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

# CI targets (uses docker for cleanup to handle CI environment permissions)
ci-clean:
	docker run --rm -v $(PWD):/work alpine rm -rf /work/data

ci-up: ci-clean
	@echo "🔼 Starting CI containers with latest pelacli..."
	docker compose pull pelacli
	docker compose up -d --build

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
	$(MAKE) logs; \
	exit 1

ci-test: ci-up ci-wait-healthy
	@echo "🧪 Running integration tests..."
	./test_txns.sh
	@echo "✅ CI integration test passed!"
