.PHONY: 
	generate-proto 
	generate-config 
	generate-config-c 
	validate-config 
	build-pathfinder 
	vulncheck-all 
	lint-all 
	lint-go 
	lint-js 
	vulncheck-js 
	vulncheck-go

# Generate the protobuf files for the RPC server and client app
generate-proto:
	@echo "Generating protobuf files for the rpc server..."
	cd proto && \
	buf generate && \
	buf generate --template buf.gen.osmosis.yaml
	@echo "Protobuf files generated successfully!"


# Generate the config files for the client app and pathfinder backend
generate-config:
	@echo "Generating config file for the client app and pathfinder backend..."
	go run config_manager/cmd/generate/main.go \
		-input ./chain_configs \
		-copy-icons ./ibc_app/public/ 

# Generate the config file for the client app and pathfinder using the already stored ibc and keplr registry
generate-config-l:
	@echo "Generating config file for the client app and pathfinder backend using the already stored ibc and keplr registry..."
	go run config_manager/cmd/generate/main.go \
		-input ./chain_configs \
		-local-registry-cache ./ibc-registry \
		-local-keplr-cache ./keplr-registry \
		-use-local-data \
		-copy-icons ./ibc_app/public/

# Validate the config files for the client app and pathfinder backend
validate-config:
	@echo "Validating chain configs..."
	go run config_manager/cmd/generate/main.go \
		-input ./chain_configs \
		-validate-only

# Build the pathfinder rpc binary
build-pathfinder:
	@echo "Building pathfinder rpc binary..."
	go build -ldflags="-s -w" -o build/pathfinder-rpc ./pathfinder/cmd/main.go
	@echo "Pathfinder rpc binary built successfully!"

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
	bun audit
	@echo "Js files vulnerability checked successfully!"

# This check requires the vulncheck cli to be installed
vulncheck-all:
	@echo "Vulnerability checking all files..."
	vulncheck ./...
	cd ibc_app && \
	bun audit
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

# This check requires the snyk cli to be installed
snyk-local:
	@echo "Vulnerability checking all files with snyk..."
	snyk test --all-projects
	@echo "All files vulnerability checked successfully with snyk!"
