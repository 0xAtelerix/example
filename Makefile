
run:
	GOPRIVATE=github.com/0xAtelerix/* go run cmd/main.go
get:
	GOPRIVATE=github.com/0xAtelerix/* go mod download github.com/0xAtelerix/sdk/gosdk

deps:
	GOPRIVATE=github.com/0xAtelerix/* go mod download

tidy:
	GOPRIVATE=github.com/0xAtelerix/* go mod tidy

env:
	go env -w GOPRIVATE=github.com/0xAtelerix/sdk