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
