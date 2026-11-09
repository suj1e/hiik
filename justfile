set shell := ["bash", "-c"]

# Fetch the modules pinned in go.mod. A local mygo checkout is not linked.
setup:
    cd apps/desktop && go mod download

fmt:
    cd apps/desktop && gofmt -w .
    cd scripts && gofmt -w .

# Regenerate the app icon from scripts/gen-icon.
icon:
    cd scripts && go run ./gen-icon

# Vet, test, check and cross-build what CI builds.
check:
    cd apps/desktop && go vet ./...
    cd apps/desktop && go test ./...
    cd scripts && go vet ./...
    cd scripts && go test ./...
    cd scripts && go run ./check-docs
    cd scripts && go run ./check-contracts
    cd scripts && go run ./check-version
    cd apps/desktop && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...
    cd apps/desktop && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./...
    cd apps/desktop && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...

dev:
    cd apps/desktop && go tool mygo dev

build:
    cd apps/desktop && go tool mygo build

release:
    cd apps/desktop && go tool mygo build -platform darwin/universal,windows/amd64,linux/amd64

# Check a built release for its Windows/Linux artifacts and print the manual
# runtime steps. No signing, installing or launching.
accept:
    cd scripts && go run ./accept-runtime
