# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Build and Run
```bash
# Development mode (with hot reload)
wails dev

# Production build
wails build

# Frontend only (in development mode)
cd frontend && npm run dev

# Frontend build
cd frontend && npm run build
```

### Testing
```bash
# Run all Go tests
go test ./...

# Run tests for specific package
go test ./pkg/sshclient -v

# Run tests with coverage
go test -cover ./...

# Run frontend tests
cd frontend && npm test
```

### Wails Development
```bash
# Install Wails CLI (if not installed)
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

## Architecture Overview

OpsCopilot is a Wails v2 desktop application combining a Go backend with React frontend for SSH terminal management and AI-assisted operations.

### Core Architecture Pattern

**Wails Bridge Layer**: The app uses Wails v2 for Go↔JavaScript bidirectional communication:
- Go methods are automatically bound to JavaScript (`window.go.main.App.*`)
- Events are emitted from Go and subscribed to in frontend via `runtime.EventsOn/EventsEmit`

### Backend Structure (Go)

**Entry Point** (`main.go`, `app.go`):
- `App` struct is the main controller, managing all subsystems
- `startup()` initializes logging and context
- `beforeClose()` handles application shutdown confirmation
- Exported methods are callable from frontend

**Key Subsystems**:

1. **SSH Client** (`pkg/sshclient/`):
   - `Client` wraps `golang.org/x/crypto/ssh` for SSH connections
   - Supports bastion/jump hosts with dual fallback strategy (TCP forwarding → netcat/console mode)
   - `StartShell()` creates PTY with stdin/stdout pipes
   - `StartShellWithSudo()` adds automatic `su -` and password handling
   - `SudoHandler` detects password prompts and auto-responds

2. **Session Management** (`pkg/session/`, `pkg/sessionmanager/`):
   - `session.Manager`: Runtime sessions (active connections)
   - `sessionmanager.Manager`: Persistent sessions (saved connections)
   - Each session tracks: SSH client, stdin pipe, SSH session (for resize)

3. **AI Service** (`pkg/ai/`, `pkg/llm/`):
   - `AIService` orchestrates LLM calls for:
     - `ParseConnectIntent()`: Natural language → SSH configs (JSON output)
     - `AskWithContext()`: RAG-based Q&A with knowledge base
     - `AskTroubleshoot()`: Structured troubleshooting steps + commands
     - `GenerateConclusion()`: Session summary generation
   - `llm.Provider` interface abstracts LLM API (uses OpenAI-compatible protocol)
   - Prompts are configurable via `config.Manager`

4. **Configuration** (`pkg/config/`):
   - `Manager` loads/saves configuration split across files:
     - `config.json`: LLM, log, docs settings
     - `prompts.json`: AI prompt templates
     - `quick_commands.json`: Quick command library
   - Defaults defined in `pkg/config/defaults.go`

5. **Session Recording** (`pkg/session_recorder/`):
   - `Recorder` tracks troubleshooting sessions with event sourcing
   - `LineBuffer` handles ANSI escape sequences for clean input capture
   - Timeline events: `terminal_input`, `terminal_output`, `note`
   - Supports broadcast deduplication (multiple sessions, same command)
   - Saves sessions as JSON + appends to `troubleshooting_history.md`

6. **Terminal Processing** (`pkg/terminal/`):
   - `LineBuffer`: Processes terminal input character-by-character
   - Handles ANSI escape codes, backspace, line editing
   - Commits lines only on Enter (clean command recording)

7. **Knowledge Base** (`pkg/knowledge/`):
   - `LoadAll()` reads all `.md` files from configured directory
   - Used as RAG context for AI Q&A

8. **Secret Storage** (`pkg/secretstore/`):
   - Uses OS keyring (Windows Credential Manager, macOS Keychain)
   - Stores SSH passwords per host:user

### Frontend Structure (React/TypeScript)

**Component Hierarchy**:
```
App.tsx (root state)
├── LayoutManager (Tab/Grid mode switching)
│   └── Terminal (xterm.js instance)
├── Sidebar (sessions/troubleshoot/chat tabs)
│   ├── SessionManager
│   ├── TroubleshootingPanel
│   └── AIChatPanel
├── QuickCommandDrawer
├── SmartConnectModal (AI-powered connection wizard)
├── SettingsModal
└── ConfirmCloseModal
```

**State Management**:
- `App.tsx` holds global state: terminals, layout mode, broadcast mode, sidebar state
- `terminalRefs`: Map of `sessionID → TerminalRef` for writing data
- `unlisteners`: Cleanup for Wails event subscriptions

**Wails Integration Patterns**:
```typescript
// Call Go method
await window.go.main.App.Connect(config);

// Subscribe to Go events
window.runtime.EventsOn("terminal-data:" + sessionId, (data) => {
  terminalRef.write(data);
});

// Emit event to Go (rare, mostly Go→Frontend)
```

### Event Flow: Terminal Connection

1. User clicks "+ 新建连接" → `SmartConnectModal` opens
2. User enters natural language → calls `ParseIntent()` → AI returns JSON configs
3. User confirms → calls `Connect(config)` for each config
4. Backend:
   - Creates SSH client (with optional bastion)
   - Starts shell session
   - Adds to `sessionMgr`
   - Spawns goroutine reading stdout → emits `terminal-data:sessionId` events
5. Frontend:
   - Receives `ConnectResult` with `sessionId`
   - Creates `TerminalSession` in state
   - Subscribes to `terminal-data:sessionId` events
   - Writes data to xterm.js instance

### Event Flow: Broadcast Mode

1. User toggles broadcast in Settings → `isBroadcastMode = true`
2. All active terminals added to `broadcastIds`
3. User types in any terminal → `handleTerminalData(id, data)`
4. If broadcast mode + id in broadcastIds → calls `Broadcast(broadcastIds, data)`
5. Backend writes to all specified stdin pipes simultaneously

### Session Recording Flow

1. User clicks "开始排查" in TroubleshootingPanel → calls `StartSession(problem)`
2. Backend creates `TroubleshootingSession` with ID, timeline
3. Terminal input/output automatically recorded via `recordInput()` calls
4. User types command → `LineBuffer` accumulates characters
5. Enter pressed → line committed → `AddEvent("terminal_input", committedLine)`
6. User clicks "结束排查" → calls `StopSession(rootCause, conclusion)`
7. Backend:
   - Saves session JSON to `sessions/session_{id}.json`
   - Appends formatted conclusion to `docs/troubleshooting_history.md`

## Configuration Files

- `wails.json`: Wails project config (frontend install/build commands)
- `config.json`: Main app config (LLM API, paths)
- `prompts.json`: AI prompt templates for different tasks
- `quick_commands.json`: Quick command library
- `sessions.json`: Saved SSH session configurations

## Key Implementation Details

### SSH Bastion Fallback
The SSH client implements a robust bastion connection strategy:
1. First tries SSH TCP forwarding (`client.Dial()`)
2. Falls back to "netcat mode" if `AllowTcpForwarding=no` on bastion
3. Executes `nc host port` via bastion console as last resort

### ANSI Escape Sequence Handling
Terminal input must be cleaned before recording:
- Raw PTY output contains ANSI codes (cursor movement, colors)
- `LineBuffer` processes character stream, handles backspace, editing
- Only commits clean lines on Enter, ignoring navigation keys

### Wails Event Naming Convention
- `session-closed`: Broadcast when SSH disconnects
- `terminal-data:{sessionId}`: Per-session stdout stream
- `confirm-close`: Custom close confirmation dialog trigger

### Dev Mode Logging
Set `OPSCOPILOT_DEV_MODE=true` environment variable to enable console + file logging during development.

## Important File Locations

- `pkg/ai/intent.go`: AI prompt templates for connection parsing
- `pkg/sshclient/client.go`: Core SSH connection logic
- `pkg/config/defaults.go`: Default prompts and configurations
- `frontend/src/App.tsx`: Main application state and event wiring
- `frontend/src/components/Terminal/Terminal.tsx`: xterm.js integration

## Common Patterns

### Adding a New Backend Method
1. Add method to `App` struct in `app.go`
2. Export method (capitalized first letter)
3. Rebuild frontend bindings: `wails dev` (auto-generates)
4. Call from frontend: `window.go.main.App.NewMethod()`

### Emitting Events from Backend
```go
runtime.EventsEmit(a.ctx, "event-name", data)
```

### Frontend Event Subscription
```typescript
useEffect(() => {
  const cancel = window.runtime.EventsOn("event-name", (data) => {
    // handle event
  });
  return () => cancel(); // cleanup
}, []);
```
