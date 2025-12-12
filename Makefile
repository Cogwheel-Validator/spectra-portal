.PHONY: 
	generate-proto 
	generate-config 
	generate-config-c 
	validate-config 
	build-solver 
	vulncheck-all 
	lint-all 
	lint-go 
	lint-js 
	vulncheck-js 
	vulncheck-go

# Generate the protobuf files for the rpc server
generate-proto:
	@echo "Generating protobuf files for the rpc server..."
	cd solver/rpc/buf && \
	buf generate 


# Generate the config files for the client app and solver backend
generate-config:
	@echo "Generating config file for the client app and solver backend..."
	go run config_manager/cmd/generate/main.go \
		-input ./chain_configs 

# Generate the config file for the client app and sovler using the already cached ibc registry
generate-config-c:
	@echo "Generating config file for the client app and solver backend using the already cached ibc registry..."
	go run config_manager/cmd/generate/main.go \
		-input ./chain_configs \
		-registry-cache ./ibc-registry \
		-use-cache 

# Validate the config files for the client app and solver backend
validate-config:
	@echo "Validating chain configs..."
	go run config_manager/cmd/generate/main.go \
		-input ./chain_configs \
		-validate-only

# Build the solver rpc binary
build-solver:
	@echo "Building solver rpc binary..."
	go build -ldflags="-s -w" -o build/solver-rpc ./solver/cmd/main.go
	@echo "Solver rpc binary built successfully!"

# This check requires the golangci-lint cli to be installed
lint-all:
	@echo "Linting all files..."
	golangci-lint run ./... && \
	cd ibc_app && \
	bun run lint
	@echo "All files linted successfully!"

# This check requires the golangci-lint cli to be installed
lint-go:
	@echo "Linting go files..."
	golangci-lint run ./...
	@echo "Go files linted successfully!"

lint-js:
	@echo "Linting js files..."
	cd ibc_app && \
	bun run lint
	@echo "Js files linted successfully!"

vulncheck-js:
	@echo "Vulnerability checking js files..."
	cd ibc_app && \
	bun run audit
	@echo "Js files vulnerability checked successfully!"

# This check requires the vulncheck cli to be installed
vulncheck-all:
	@echo "Vulnerability checking all files..."
	vulncheck ./...
	cd ibc_app && \
	bun run audit
	@echo "All files vulnerability checked successfully!"

# This check requires the semgrep cli to be installed
# And it also requires to have an account
vulncheck-semgrep-ci:
	@echo "Vulnerability checking all files with semgrep..."
	semgrep ci
	@echo "All files vulnerability checked successfully with semgrep!"

# This check requires the semgrep cli to be installed
vulncheck-semgrep-local:
	@echo "Vulnerability checking all files with semgrep..."
	semgrep scan
	@echo "All files vulnerability checked successfully with semgrep!"

# This check requires the govulncheck cli to be installed
vulncheck-go:
	@echo "Vulnerability checking go files..."
	govulncheck ./...
	@echo "Go files vulnerability checked successfully!"