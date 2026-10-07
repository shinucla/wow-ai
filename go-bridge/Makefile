.PHONY: build-win build-whisper-client build-party-client run-whisper run-party test clean

# Cross-compile a Windows amd64 executable (works from Linux/WSL/macOS/Windows with Go).
build-win:
	GOOS=windows GOARCH=amd64 go build -o wow-ai-bridge.exe ./cmd/wow-ai-bridge

# Local whisper REPL / one-shot tester (same coach prompt + parse as the bridge).
build-whisper-client:
	go build -o go-whisper-client ./cmd/go-whisper-client

# Local party REPL / one-shot tester (party coach prompt + parse).
build-party-client:
	go build -o go-party-client ./cmd/go-party-client

# Run whisper client in WSL using the Windows AppData config (saved API key).
# Interactive:  make run-whisper
# One-shot:     make run-whisper ARGS='are you an ai bot?'
run-whisper: build-whisper-client
	@$(MAKE) --no-print-directory _run-client BIN=go-whisper-client

# Run party client in WSL. Incoming lines look like: Name: message
# Interactive:  make run-party
# One-shot:     make run-party ARGS='Bob: ready?'
run-party: build-party-client
	@$(MAKE) --no-print-directory _run-client BIN=go-party-client

_run-client:
	@cfg="$(WOW_AI_GO_CONFIG)"; \
	if [ -z "$$cfg" ]; then \
	  cfg=$$(ls /mnt/c/Users/*/AppData/Roaming/wow-ai-bridge/config.json 2>/dev/null | head -1); \
	fi; \
	if [ -z "$$cfg" ] || [ ! -f "$$cfg" ]; then \
	  echo "No config found. Set WOW_AI_GO_CONFIG or save an API key in the bridge Settings UI."; \
	  exit 1; \
	fi; \
	echo "Using config: $$cfg"; \
	WOW_AI_GO_CONFIG="$$cfg" ./$(BIN) $(ARGS)

test:
	go test ./...

clean:
	rm -f wow-ai-bridge.exe go-whisper-client go-party-client
