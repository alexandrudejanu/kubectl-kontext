PLUGIN_NAME=kubectl-kontext
PLATFORMS=darwin-arm64 linux-arm64

.PHONY: all archives sha release clean remove-index install-mcp go-build go-run compare-go

all: archives sha

archives:
	@for platform in $(PLATFORMS); do \
	  tar -czf $(PLUGIN_NAME)-$$platform.tar.gz $(PLUGIN_NAME); \
	done

sha:
	@for platform in $(PLATFORMS); do \
	  echo "$$platform:"; \
	  shasum -a 256 $(PLUGIN_NAME)-$$platform.tar.gz | awk '{print $$1}'; \
	done

release: archives sha

go-build:
	@mkdir -p bin
	@go build -o bin/kubectl-kontext ./cmd/kubectl-kontext-go
	@echo "Add $$(pwd)/bin to your PATH"
	@echo 'export PATH="$$PATH:$$(pwd)/bin"'

install-mcp:
	@mkdir -p $(HOME)/.local/bin/kubectl-kontext
	@cp kubectl-kontext $(HOME)/.local/bin/kubectl-kontext/kubectl-kontext
	@cp local_mpc_setup/mcp_server.py $(HOME)/.local/bin/kubectl-kontext/mcp_server.py
	@chmod +x $(HOME)/.local/bin/kubectl-kontext/kubectl-kontext
	@echo "Installed to $(HOME)/.local/bin/kubectl-kontext/"
	@echo "Update claude_desktop_config.json args to: $(HOME)/.local/bin/kubectl-kontext/mcp_server.py"
	@echo "Restart Claude Desktop to pick up changes"

remove-index:
	kubectl krew uninstall kontext
	kubectl krew index remove my-index

clean:
	rm -f $(PLUGIN_NAME)-*.tar.gz
	rm -f bin/kubectl-kontext