# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

OpsCopilot is a Wails-based desktop application for SSH terminal management and AI-powered troubleshooting. It combines a Go backend with a React frontend to provide:

- Multi-session SSH terminal management with bastion host support
- AI-powered Q&A and troubleshooting using local knowledge bases
- Session recording for troubleshooting workflows
- File transfer (SFTP/SCP) with progress tracking
- Java process monitoring capabilities

## Development Commands

### Running the Application
```bash
# Development mode (hot reload, console + file logging)
start_dev.bat

# Or manually with environment variables:
set OPSCOPILOT_DEV_MODE=true
set LLM_BASE_URL=https://your-api-endpoint/v1
set LLM_MODEL=your-model-name
set LLM_API_KEY=your-api-key
wails dev
```

### Building
```bash
# Production build (file logging only)
build_release.bat

# Or manually:
wails build
```

### Testing
```bash
# Go tests
go test ./pkg/...

# Frontend tests
cd frontend && npm test
```

### Frontend Development
```bash
cd frontend
npm install
npm run dev    # Vite dev server
npm run build  # Production build
```

## Architecture

### Backend Structure (Go)

**Main Application (`app.go`)**
- Central coordinator that binds to Wails frontend
- Manages SSH sessions, AI services, configuration, and recording
- All exported methods are callable from frontend via Wails binding

**Key Packages:**
- `pkg/sshclient` - SSH client with bastion/jump host support, handles direct connections and netcat fallback
- `pkg/session` - In-memory SSH session management (terminal sessions)
- `pkg/sessionmanager` - Persistent saved session storage (SSH connection profiles)
- `pkg/session_recorder` - Troubleshooting workflow recording with timeline events
- `pkg/ai` - AI service with ReAct agent loop for knowledge base queries
- `pkg/llm` - LLM provider abstraction (OpenAI-compatible)
- `pkg/config` - Configuration management with migration support
- `pkg/knowledge` - Local documentation search with vector-like relevance scoring
- `pkg/filetransfer` - SFTP/SCP file operations with automatic fallback
- `pkg/javamonitor` - Java process/thread monitoring via jstack
- `pkg/completion` - Command completion database
- `pkg/secretstore` - OS keyring integration for password storage
- `pkg/terminal` - Terminal line buffer for ANSI code handling

### Frontend Structure (React/TypeScript)

**Key Components:**
- `App.tsx` - Main application layout
- `components/Terminal` - xterm.js-based terminal with xterm addons (fit, search)
- `components/Sidebar` - Connection management, saved sessions, Java monitoring
- `components/TroubleshootingPanel` - AI troubleshooting workflow UI
- `components/SettingsModal` - Configuration management UI
- `wailsjs/` - Auto-generated bindings to Go backend

### Data Flow

**SSH Connection:**
1. Frontend calls `Connect(config)` with host/user/password
2. Backend creates SSH client, starts shell with PTY
3. Session ID returned to frontend
4. Backend goroutine reads stdout and emits `terminal-data:{sessionId}` events
5. Frontend sends input via `Write(sessionID, data)`
6. On close, backend emits `session-closed` event

**AI Troubleshooting:**
1. Frontend calls `AskTroubleshoot(problem, repoPath, customVars)`
2. Backend runs parallel queries: OpsCopilot AI + optional external script
3. Results integrated via AI synthesis
4. Returns structured JSON with ready-to-use steps and commands

**File Transfer:**
1. Frontend calls `FTUpload` or `FTDownload`
2. Returns task ID immediately
3. Progress updates via `file-transfer-progress` events
4. Completion via `file-transfer-done` event

## Configuration

Configuration files (in executable directory or working directory):
- `config.json` - Main application settings
- `sessions.json` - Saved SSH sessions
- `prompts.json` - AI prompt templates
- `quick_commands.json` - Quick command snippets
- `highlight_rules.json` - Terminal syntax highlighting rules

**Environment Variables:**
- `LLM_BASE_URL` - LLM API endpoint
- `LLM_MODEL` - Model name
- `LLM_API_KEY` - API key
- `OPSCOPILOT_DEV_MODE` - Enable console logging (default: false in production)

## Important Implementation Details

### SSH Connection Handling
- Supports bastion/jump hosts with automatic TCP forwarding → netcat fallback
- Password authentication via keyboard-interactive for compatibility
- Root password support for sudo auto-elevation
- Session auto-save to `sessions.json` on successful connection

### Knowledge Base Integration
- Searches markdown files in configured `docs.dir` (defaults to `./docs` or `./knowledge`)
- Uses weighted term frequency scoring for relevance
- ReAct agent loop: search → list files → read → answer
- Tool definitions in `pkg/knowledge/tools.go`

### Terminal Input Recording
- Uses `LineBuffer` to handle ANSI escape codes and line editing
- Only records committed lines (Enter key), not partial input
- Broadcast commands deduplicated across multiple sessions

### File Transfer Mode Detection
- Auto-detects SFTP vs SCP support
- Falls back from SFTP to SCP if SFTP subsystem unavailable
- Uses root client if root password provided, otherwise login user

### Session Lifecycle
- `beforeClose` hook checks for active sessions before allowing quit
- Force quit flag skips confirmation
- Active sessions include both terminals and troubleshooting recordings

## Testing Notes

- SSH tests use `github.com/golang/crypto/ssh/test` server
- File transfer tests mock SSH with test server
- Java monitoring tests use sample jstack output
- Frontend tests use vitest + jsdom

## Common Patterns

**Adding a new AI-powered feature:**
1. Add prompt template to `pkg/config/defaults.go`
2. Add method to `pkg/ai/agent.go` or create new service method
3. Expose via `app.go` with JSON return for frontend
4. Emit `agent:status` events for progress feedback

**Adding frontend-backend communication:**
1. Add Go method to `App` struct (exported)
2. Run `wails dev` to regenerate bindings in `frontend/wailsjs/`
3. Import and call from frontend TypeScript

**Handling streaming events to frontend:**
```go
runtime.EventsEmit(a.ctx, "event-name", data)
```
Frontend listens in useEffect with `runtime.EventsOn`.
