# Go UI Bridge (Windows)

A Windows-native companion for the WoW AI addon. Same pixel-strip in / slot-file out protocol as the Node bridge, plus a local Settings UI for LLM API keys.

## What it does

1. Captures the top-left colored squares from the WoW window (GDI).
2. Decodes them into chat messages.
3. Calls an LLM API (OpenAI, Anthropic, or any OpenAI-compatible endpoint).
4. Writes replies into `WoWAI_S###` slot files and flips signal `.wav` files.
5. Opens a local Status + Settings page in your browser.

This bridge talks to **HTTP LLM APIs** (token in Settings). It does not launch the Claude/Codex CLIs — use the Node bridge for those.

## Whisper / party short-circuit testers

Same saved API key, coach prompt, and JSON parse as in-game auto-reply — no WoW window required:

```bash
cd go-bridge
make run-whisper                              # interactive (WSL picks Windows AppData config)
make run-whisper ARGS='are you an ai bot?'    # one-shot
make run-party                                # interactive party coach
make run-party ARGS='Bob: ready for pulls?'   # one-shot (Name: message)
```

## Multi-client (two 3.3.5a windows, one install)

One game folder means one shared `Interface\AddOns`. Isolation uses **instance numbers**:

1. In Settings set **Max game clients** (e.g. `2`) → Save → **Install slots** → fully restart both WoW windows.
2. Arrange windows left → right. The bridge labels them instance **1**, **2**, …
3. In the left client: `/wow-ai instance 1`
4. In the right client: `/wow-ai instance 2`
5. Start the bridge. Each client’s pixel strip is captured from its own window; replies and signal `.wav` files go under `WoWAI\i01\` / `WoWAI\i02\` and slot addons `WoWAI_I01_S###` / `WoWAI_I02_S###`.

A must not receive B’s replies: different slot trees and ack/sig paths, plus an `instance` field on each reply record.

## Build (on Windows)

```powershell
cd go-bridge
go test ./...
go build -o wow-ai-bridge.exe ./cmd/wow-ai-bridge
```

Cross-compile from Linux/WSL (capture only works when the exe runs on Windows):

```bash
cd go-bridge
GOOS=windows GOARCH=amd64 go build -o wow-ai-bridge.exe ./cmd/wow-ai-bridge
```

## First run

1. Copy the `addon/WoWAI` folder into your client's `Interface\AddOns\WoWAI` (or use your existing install).
2. Start `wow-ai-bridge.exe` — a browser tab opens.
3. Open **Settings**:
   - **AddOns folder** → e.g. `C:\Games\World of Warcraft - WOTLK 3.3.5a\Interface\AddOns`
   - **Process name** → usually `Wow` (no `.exe`) for 3.3.5a private servers
   - **TOC Interface** → `30300` for 3.3.5a
   - **Provider / API key / model**
4. Click **Save settings**, then **Install slots**, then fully quit and relaunch WoW.
5. Click **Start**. Enable the WoW AI addon in-game and send a message.

Config is saved under `%AppData%\wow-ai-bridge\config.json` (file mode restricted). Override with `-config path.json` or env `WOW_AI_GO_CONFIG`.

## Flags

| Flag | Meaning |
|------|---------|
| (none) | Open browser UI |
| `-headless` | No UI; start capture immediately |
| `-install-slots` | Create slot pool + signal wavs, then exit |
| `-inject "hello"` | Fake an in-game message (needs API key) |
| `-config path` | Use that config file |

## Requirements

- Windows (for live capture)
- WoW windowed or borderless (not exclusive fullscreen)
- Addon already in `Interface\AddOns\WoWAI`
- An LLM API key

## Layout

```
go-bridge/
  cmd/wow-ai-bridge/     entrypoint + UI launch
  internal/codec/        pixel strip encode/decode
  internal/protocol/     strip records + Lua slot files
  internal/capture/      Windows GDI capture
  internal/llm/          OpenAI / Anthropic HTTP clients
  internal/publish/      slot writes + install-slots
  internal/signals/      ack/sig/act/presence wavs
  internal/bridge/       main loop
  internal/ui/           local Status + Settings web UI
  internal/config/       JSON settings
```
