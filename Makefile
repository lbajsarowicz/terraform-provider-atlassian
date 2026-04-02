BINARY_NAME=terraform-provider-atlassian
INSTALL_DIR=$(HOME)/.terraform.d/plugins/registry.opentofu.org/lbajsarowicz/atlassian/0.1.0/$$(go env GOOS)_$$(go env GOARCH)

.PHONY: build install test testacc lint clean testintegration sweep

build:
	go build -o $(BINARY_NAME)

install: build
	mkdir -p $(INSTALL_DIR)
	cp $(BINARY_NAME) $(INSTALL_DIR)/

test:
	go test ./... -v -count=1

testacc:
	TF_ACC=1 go test ./... -v -count=1 -timeout 10m

lint:
	golangci-lint run ./...

clean:
	rm -f $(BINARY_NAME)

testintegration: ## Run integration tests against real Jira (requires ATLASSIAN_URL, ATLASSIAN_USER, ATLASSIAN_TOKEN, ATLASSIAN_SWEEP_CONFIRM=<host>)
	TF_ACC=1 go test ./internal/jira/... -v -count=1 -timeout 30m -run '^TestIntegration'

sweep: ## Clean up stale tf-acc-test-* resources (requires ATLASSIAN_SWEEP_CONFIRM=<host>)
	go test ./internal/jira/... -v -sweep=all -timeout 10m
