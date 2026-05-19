APP_NAME    := teams-music
BUILD_DIR   := ./build
BINARY      := $(BUILD_DIR)/$(APP_NAME)
INSTALL_DIR := /usr/local/bin

.PHONY: build install uninstall clean init agent-install agent-uninstall agent-status logs

# ─── Build ─────────────────────────────────────────────────

build:
	@echo "🔨 Building $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build -o $(BINARY) ./cmd/teams-music/
	@echo "✅ Binary: $(BINARY)"

# ─── Install / Uninstall Binary ────────────────────────────

install: build
	@echo "📦 Installing to $(INSTALL_DIR)/$(APP_NAME)..."
	@cp $(BINARY) $(INSTALL_DIR)/$(APP_NAME)
	@chmod 755 $(INSTALL_DIR)/$(APP_NAME)
	@echo "✅ Installed: $(INSTALL_DIR)/$(APP_NAME)"

uninstall: agent-uninstall
	@echo "🗑️  Removing $(INSTALL_DIR)/$(APP_NAME)..."
	@rm -f $(INSTALL_DIR)/$(APP_NAME)
	@echo "✅ Uninstalled"

# ─── Config ────────────────────────────────────────────────

init: build
	@$(BINARY) --init

# ─── LaunchAgent ───────────────────────────────────────────

# Set up LaunchAgent (NO sudo – runs as your user)
agent-install:
	@if [ ! -f $(INSTALL_DIR)/$(APP_NAME) ]; then \
		echo "❌ Binary not found. Run 'sudo make install' first."; \
		exit 1; \
	fi
	@$(INSTALL_DIR)/$(APP_NAME) --install

agent-uninstall:
	@$(INSTALL_DIR)/$(APP_NAME) --uninstall 2>/dev/null || true

agent-status:
	@$(INSTALL_DIR)/$(APP_NAME) --status

# ─── Logs ──────────────────────────────────────────────────

logs:
	@tail -f ~/Library/Logs/teams-music.out.log

logs-err:
	@tail -f ~/Library/Logs/teams-music.err.log

# ─── Clean ─────────────────────────────────────────────────

clean:
	@rm -rf $(BUILD_DIR)
	@echo "✅ Build directory cleaned"