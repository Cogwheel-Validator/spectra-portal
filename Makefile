.PHONY:
	setup
	setup-hooks
	generate-proto
	generate-config
	generate-config-c
	validate-config
	build-pathfinder
	build-client
	vulncheck-all
	lint-all
	lint-go
	lint-js
	lint-commits
	vulncheck-js
	vulncheck-go

# Sets up everything needed for local development
setup: setup-hooks
	@echo "Development environment set up successfully!"
	# Add later more setup steps

# Installs the git hooks (commit-msg linting, etc.) via lefthook
# This check requires the following clis to be installed:
#   - lefthook:   https://github.com/evilmartians/lefthook (go install github.com/evilmartians/lefthook/v2@latest)
#   - cocogitto:  https://github.com/cocogitto/cocogitto (cargo install --locked cocogitto)
#   - koji:       https://github.com/cococonscious/koji (cargo install --locked koji)
#   - typos:      https://github.com/crate-ci/typos (cargo install --locked typos-cli)
setup-hooks:
	@echo "Installing git hooks..."
	lefthook install
	@echo "Git hooks installed successfully!"

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
		-copy-icons ./client_app/public/

# Generate the config file for the client app and pathfinder using the already stored ibc and keplr registry
generate-config-l:
	@echo "Generating config file for the client app and pathfinder backend using the already stored ibc and keplr registry..."
	go run config_manager/cmd/generate/main.go \
		-input ./chain_configs \
		-local-registry-cache ./ibc-registry \
		-local-keplr-cache ./keplr-registry \
		-use-local-data \
		-copy-icons ./client_app/public/

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

build-client:
	@echo "Building the client app..."
	cd client_app && pnpm install --frozen-lockfile && pnpm run build
	@echo "Client app built successfully!"

# This check requires the golangci-lint cli to be installed
lint-all:
	@echo "Linting all files..."
	golangci-lint run ./... && \
	cd client_app && \
	pnpm run lint
	@echo "All files linted successfully!"

# This check requires the golangci-lint cli to be installed
lint-go:
	@echo "Linting go files..."
	golangci-lint run ./...
	@echo "Go files linted successfully!"

lint-js:
	@echo "Linting js files..."
	cd client_app && \
	pnpm run lint
	@echo "Js files linted successfully!"

# Lints commit messages on the current branch against Conventional Commits
# This check requires the cocogitto and typos clis to be installed (see setup-hooks)
lint-commits:
	@echo "Linting commit messages..."
	cog check
	@for sha in $$(git rev-list HEAD); do \
		git show -s --format=%B "$$sha" > /tmp/spectra-commit-msg.txt; \
		scripts/require-scope.sh /tmp/spectra-commit-msg.txt || exit 1; \
	done
	git log --format=%B > /tmp/spectra-commit-msgs.txt
	typos /tmp/spectra-commit-msgs.txt
	@echo "Commit messages linted successfully!"

vulncheck-js:
	@echo "Vulnerability checking js files..."
	cd client_app && \
	pnpm audit
	@echo "Js files vulnerability checked successfully!"

# This check requires the vulncheck cli to be installed
vulncheck-all:
	@echo "Vulnerability checking all files..."
	govulncheck ./...
	cd client_app && \
	pnpm audit
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
