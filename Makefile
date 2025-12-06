.PHONY: generate-proto generate-config generate-config-c validate-config build-solver vulncheck lint

# Generate the protobuf files for the rpc server
generate-proto:
	@echo "Generating protobuf files for the rpc server..."
	cd solver/rpc/buf && \
	buf generate 


# Generate the config files for the client app and solver backend
generate-config:
	@echo "Generating config file for the client app and solver backend..."
	cd config_manager/cmd/generate && \
	go run main.go \
		-input ../chain_configs \

# Generate the config file for the client app and sovler using the already cached ibc registry
generate-config-c:
	@echo "Generating config file for the client app and solver backend using the already cached ibc registry..."
	cd config_manager/cmd/generate && \
	go run main.go \
		-input ../chain_configs \
		-registry-cache ../ibc-registry \
		-use-cache 

# Validate the config files for the client app and solver backend
validate-config:
	@echo "Validating chain configs..."
	cd config_manager/cmd/generate && \
	go run main.go \
		-input ../chain_configs \
		-validate-only

# Build the solver rpc binary
build-solver:
	@echo "Building solver rpc binary..."
	go build -ldflags="-s -w" -o build/solver-rpc ./solver/cmd/main.go
	@echo "Solver rpc binary built successfully!"

lint:
	golangci-lint run ./...

vulncheck:
	govulncheck ./...