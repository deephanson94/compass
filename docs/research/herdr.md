# Herdr: Comprehensive Technical Research Report

## 1. Core Value Proposition & Features

### What Herdr Does
Herdr is a **terminal multiplexer and session manager purpose-built for AI coding agents**. It functions as a background server that manages terminal sessions, allowing agents to remain "always running" even when disconnected. Users can reattach from any terminal or via SSH.

### Core Value Proposition
- **Always Running**: Close the lid, drop network, or restart the machine; agents keep working and sessions come back
- **Agent Awareness**: Every pane is marked working, blocked, or idle. When an agent stops and needs an answer, herdr explicitly says so
- **Agent-Native**: Agents drive herdr through CLI and socket API
- **Universal Compatibility**: Works with Claude Code, Codex, Cursor, Copilot, Grok, and other tools without wrapping them
- **Dual Input Support**: Both keyboard shortcuts (tmux-style prefix keys) and mouse interactions simultaneously
- **Plugins**: Extensible via marketplace

### Main Features (v0.8.2)
- **Persistent Sessions**: Agents continue in background; reattach via `herdr` command or SSH
- **Agent State Detection**: Three-state marking (working/blocked/idle) via terminal tail pattern matching
- **Status Indicators**: Configurable visual indicators (dots or symbols) showing agent lifecycle
- **Multi-agent Support**: Multiple concurrent agents detected (Claude, Codex, Cursor, Devin, Cline, Pi, OpenCode, Copilot, Kimi, Kiro, Droid, Grok, Qwen, Maki, Muse, and more)
- **Layout Management**: BSP (binary space partition) tree layout for tiling panes within workspaces
- **Git Integration**: Auto-labeling workspaces from git branches and status
- **Session Persistence**: Resume sessions after reconnect with full state preserved
- **Remote Access**: Windows clients can attach to Linux/macOS servers via `--remote`
- **Keyboard Enhancement**: Support for Kitty keyboard protocol, IME compatibility, modifyOtherKeys
- **Pane Graphics**: Support for Kitty/Sixel image rendering, PNG detection
- **Mouse Support**: Full mouse reporting, drag-resize splits, click navigation
- **Configuration**: TOML-based config for themes, keybindings, sidebar layout, status indicators
- **Plugins**: Extend panes and workflows via marketplace


## 2. Architecture

### Language & Framework
- **Language**: Rust (edition 2021)
- **Primary TUI Framework**: `ratatui` v0.30 (terminal UI rendering)
- **PTY Management**: `portable-pty` (vendored custom version, pinned to 0.9.0)
- **Async Runtime**: `tokio` v1 (multi-threaded, with sync, time, process, io-util features)
- **IPC/Sockets**: `interprocess` v2.4.2 (local socket communication)
- **Terminal Control**: `crossterm` v0.29 (terminal events, mouse, keyboard, bracketed paste)
- **Input Parsing**: Custom terminal key sequence parser + regex pattern matching
- **Serialization**: `serde` + `serde_json` + JSONC support

### Dependencies Summary (key ones from Cargo.toml)
```
ratatui 0.30 (with unstable-rendered-line-info)
portable-pty 0.9.0 (PTY spawning/management)
tokio 1.x (async runtime)
crossterm 0.29 (terminal I/O)
interprocess 2.4.2 (IPC sockets)
serde/serde_json (serialization)
regex (pattern matching)
time 0.3.47 (timestamp handling)
jsonc-parser (config parsing)
```

### Architecture Overview

```
herdr (single Rust binary, ~40MB)
├── Server (background daemon)
│   ├── PTY Actor System (tokio-based)
│   │   └── portable-pty: manages child process TTYs
│   ├── API Server (IPC socket)
│   │   └── interprocess local_socket for CLI communication
│   └── State Management
│       ├── Session (session.rs)
│       ├── Workspaces (workspace.rs)
│       ├── Tabs & Panes (pane.rs, layout.rs)
│       └── Agent Detection (detect/mod.rs)
│
├── TUI Client (ratatui-based)
│   ├── Input Handler (raw_input.rs, input/)
│   ├── UI Renderer (ui.rs, ui/ modules)
│   ├── Layout Engine (layout.rs - BSP tree)
│   └── State Display
│
└── CLI (clap-based command parser)
    └── Commands for agents, panes, workspaces, tabs, worktrees
```

### Session Discovery & Attachment

#### Session Discovery Mechanism
1. **Session Identification**: Each session has a unique name (default: "default")
   - Sessions stored in `~/.config/herdr/` (Unix) or equivalent on Windows
   - Socket path: `$XDG_RUNTIME_DIR/herdr-{session}.sock` or platform equivalent
   - Env var: `HERDR_SESSION` to specify active session

2. **Attaching to Sessions**
   ```bash
   herdr                    # Attach to default session or create it
   herdr session attach <name>  # Attach to named session
   herdr --session=<name>   # Specify session via flag
   ```

3. **Session Configuration Files**
   - Stored per-session in state directory
   - Persisted workspace/tab/pane layout
   - Agent session references for resume
   - State file format: JSONL with snapshots

#### Agent Session Resume
Files: `src/agent_resume.rs`, `src/app/agent_resume.rs`

Herdr detects and tracks agent sessions via:
- **Session Refs**: Agent sessions can be referenced by:
  - `id`: Session ID (Claude Code, Cursor, etc.)
  - `path`: Session path (Pi, OMP agents using path-based recovery)
- **Supported Agents**: Claude, Codex, Cursor, Devin, Grok, Qwen, Qodercli, and others
- **Persistence**: On exit, herdr saves agent session refs to restore on reconnect
- **Restoration**: `agent_resume.rs` implements resumption logic including:
  - Validating saved session state
  - Launching agent with correct session ID/path
  - Re-establishing terminal connection

### PTY Management Architecture

**File**: `src/pty/` directory + `portable-pty` vendor

- **Per-pane PTY Actor**: Each pane has a dedicated `PtyIoActor` (async actor pattern)
- **PTY I/O Pipeline**:
  1. `PtyIoActor` spawns child process via `portable-pty::CommandBuilder`
  2. Reads PTY output continuously
  3. Sends output to terminal buffer via channel
  4. Handles PTY input from client
  5. Manages PTY size changes (SIGWINCH)
  
- **Terminal Environment Setup**:
  - Sets `TERM=xterm-256color`, `COLORTERM=truecolor`
  - Injects herdr identity vars:
    - `HERDR_ENV=1` (marks running in herdr)
    - `HERDR_SESSION`, `HERDR_WORKSPACE_ID`, `HERDR_TAB_ID`, `HERDR_PANE_ID`
  - Removes inherited terminal identity to prevent leakage over SSH

### UI Rendering

**Main files**: `src/ui.rs` (25KB), `src/ui/` modules (450KB+)

- **Framework**: ratatui v0.30
- **Layout Engine**: Binary space partition (BSP) tree
- **Rendering Components**:
  - `ui/sidebar.rs`: Workspace/agent panel with state indicators
  - `ui/tabs.rs`: Desktop tab bar with tab navigation
  - `ui/panes.rs`: Terminal pane rendering with scrollbars
  - `ui/dialogs.rs`: Modal dialogs for naming, etc.
  - `ui/status.rs`: Status indicators (working/blocked/idle icons)
  - `ui/navigator.rs`: Navigate mode for session navigation
  
- **Terminal Abstraction** (`src/terminal/`):
  - Writes to allocated terminal buffer
  - Handles cursor positioning, scrolling
  - Supports Kitty graphics protocol
  - Handles alternate screen (for TUI agents)

### API & IPC Architecture

**Files**: `src/api/`, `src/ipc.rs`

- **Transport**: Unix domain sockets (interprocess crate)
  - Socket path: `$HERDR_SOCKET_PATH` env var or computed from session dir
  - Bincode serialization for wire protocol
  
- **Request/Response Pattern**:
  - CLI sends JSON requests via `herdr` binary
  - Server processes via API schema (src/api/schema.rs)
  - Responses include result data or error messages
  
- **Commands Supported**:
  - `agent start`, `agent prompt`, `agent send-keys`, `agent list`
  - `pane split`, `pane move`, `pane send-keys`, `pane read`, `pane wait-output`
  - `workspace create`, `workspace focus`, `workspace list`
  - `tab create`, `tab focus`, `tab list`
  - `server reload-config`, `session list`
  - Streaming commands: `pane read`, `pane wait-output`


## 3. UX/Layout

### Main UI Panels & Layout

```
┌─────────────────────────────────────────────┐
│ Tab Bar (desktop mode)                      │
├──────────────┬──────────────────────────────┤
│              │                              │
│  Sidebar     │  Terminal Panes              │
│  (Workspaces│  (BSP-tiled layout)           │
│   & Agents)  │                              │
│              │  ├─ Pane 1 (focused)        │
│              │  ├─ Pane 2                   │
│              │  └─ Pane 3                   │
│              │                              │
└──────────────┴──────────────────────────────┘
```

### Sidebar Sections

**Agent Panel** (collapsible, shows live agents):
- Workspace sections (grouped by workspace)
- Under each workspace: tabs
- Under each tab: panes with agents
- Per-pane display:
  - **State Icon** (configurable: dots or symbols)
    - Blocked: `●` (red dot) or `×` (red X)
    - Working: `●` (yellow dot) or `◐` (yellow semicircle)
    - Idle (unseen): `●` (teal dot) or `✓` (teal check)
    - Idle (seen): `○` (green circle) or `○` (green circle)
  - Agent label (e.g., "claude", "cursor", "pi")
  - Terminal title
  - Optional workspace/tab context

### Panel Layout Customization

Configuration options (from `src/config/`):
- `ui.sidebar_position`: left/right/hidden
- `ui.sidebar_min_width`, `ui.sidebar_max_width`
- `ui.tab_bar_position`: top/bottom/hidden
- `ui.pane_outer_borders`: show/hide outer split borders
- `ui.status_indicators`: "dots" or "symbols" style

### Keyboard Navigation

#### Default Prefix Key
- `Ctrl+B` (tmux-style, customizable)

#### Key Categories
1. **Navigation**:
   - `prefix+n`/`prefix+p`: next/previous workspace
   - `prefix+[`/`prefix+]`: previous/next tab
   - `arrow keys`: navigate panes/sidebar
   - `hjkl`: vim-style navigation (in modal)
   - `j`/`k`: navigate agent list
   - `/`: search/filter agents

2. **Pane Management**:
   - `prefix+-`: split vertically
   - `prefix+|`: split horizontally
   - `prefix+x`: close pane
   - `prefix+z`: zoom pane
   - `prefix+{move_tab_previous}`, `prefix+{move_tab_next}`: reorder tabs
   - `prefix+{resize_pane_left|up|down|right}`: direct pane resize

3. **Mode Switching**:
   - `prefix+q`: enter navigate mode (session navigator)
   - `prefix+[`: enter copy mode (with `B`/`E`/`W` big-word motions)
   - `Esc`: exit mode

4. **Agent Control** (from CLI):
   - `herdr agent prompt <agent> <text>`: send text to agent
   - `herdr agent send-keys <agent>`: send keys
   - `herdr pane send-keys <pane>`: send keys to pane

5. **Mouse**:
   - Click panes to focus
   - Drag split borders to resize
   - Click and drag to select text
   - Right-click for pane menu
   - Scroll wheel to navigate

### State Indicators & UI Summary

#### Agent State Display (from `src/ui/status.rs`)
```rust
pub enum AgentState {
    Idle,      // Ready for input
    Working,   // Actively processing
    Blocked,   // Waiting for human input
    Unknown,   // Unrecognized program
}
```

#### State Labels & Colors
| State | Icon (Dots) | Icon (Symbols) | Label | Color | Meaning |
|-------|-------------|----------------|-------|-------|---------|
| Blocked | ● | × | blocked | red | Needs human input |
| Working | ● | ◐ | working | yellow | Processing |
| Idle (new) | ● | ✓ | done | teal | Just finished (unseen) |
| Idle (seen) | ○ | ○ | idle | green | Ready & seen |
| Unknown | · | · | idle | gray | Unrecognized |

#### Session Navigator (`navigate mode`, `prefix+q`)
- Per-workspace view with workspace name, tab count, pane count
- Per-agent view in "priority" mode (sorts by state: blocked > working > idle)
- Grouping mode: by workspace or by priority
- Live filtering with `/` key
- Quick jump to any agent/pane with arrow keys + enter

### Config File Structure

**Location**: `~/.config/herdr/herdr.toml` (Unix) or equivalent

**Main Sections**:
```toml
[keys]
prefix = "ctrl+b"  # customize prefix
move_tab_previous = "shift+h"
move_tab_next = "shift+l"
resize_pane_left = "shift+left"
# ... many more bindings

[ui]
sidebar_position = "left"
sidebar_min_width = 20
sidebar_max_width = 50
tab_bar_position = "top"
pane_scrollbars = true
status_indicators = "symbols"  # or "dots"
pane_outer_borders = "all"    # or "inside"

[theme]
name = "catppuccin"  # or custom theme

[ui.sound]
enable = true

[server]
headless_cols = 120
headless_rows = 40
```


## 4. Session State & Activity Detection

### State Detection Mechanism

**File**: `src/detect/mod.rs` (550+ lines)

Herdr detects agent state via **terminal tail pattern matching**:

1. **Per-pane Detection Loop**:
   - Periodically reads pane's terminal buffer (bottom ~100 lines)
   - Matches patterns against known agent output signatures
   - Updates state (idle/working/blocked)
   - Publishes state change to UI

2. **Pattern Matching Manifests**:
   - **File**: `src/detect/manifest.rs`, `src/detect/manifest_update.rs`
   - Contains regex patterns per agent type
   - Patterns detect:
     - Agent prompt ready (idle state)
     - Active spinner/progress indicator (working state)
     - Input request or confirmation dialog (blocked state)
   
3. **Supported Agents & Detection**:
   Herdr detects 23 agents including:
   - Claude, Codex, Cursor, Devin, Cline, Pi, OpenCode, Copilot, Grok, Qwen
   - Each with custom state detection logic
   - Example: Claude detected via title spinner half-circles; Qwen via locale-independent state strings

4. **State Publication**:
   - **Grace Window**: `AGENT_STARTUP_GRACE_WINDOW` (waits before declaring "idle" on startup)
   - **Pending Idle Confirmation**: `AGENT_PENDING_IDLE_RECHECK` (confirms idle state before publish)
   - **Duplicate Publishing**: Avoided via `ScreenDetectionPublishDecision` logic
   - **Visible Chrome**: Separate detection for UI chrome vs actual state

5. **PTY Activity Monitoring** (Fallback):
   - When terminal tail patterns aren't conclusive
   - Monitors recent PTY output activity
   - Working = recent output; Idle = no output

### Detection Data Structures

```rust
pub struct AgentDetection {
    pub state: AgentState,
    pub skip_state_update: bool,        // Don't update if viewing history
    pub visible_idle: bool,              // UI shows idle prompt
    pub visible_blocker: bool,           // UI shows input dialog
    pub visible_working: bool,           // UI shows activity indicator
}
```

### Seen/Unseen State Tracking

- **Seen**: User has focused the tab/pane in Herdr UI
- **Unseen**: Task finished but user hasn't viewed it yet
- Affects display:
  - Unseen idle → labeled "done", teal color
  - Seen idle → labeled "idle", green color

### Activity Confidence Arbitration

**File**: `src/pane/agent_detection.rs`

Multiple sources of state signals:
1. Screen tail pattern (high confidence, agent-specific)
2. PTY activity (working when receiving output)
3. Visible UI chrome (blocked if approval UI detected)
4. Integration state (from agent protocol/API)

**Arbitration**: Screen-based detection overrides PTY activity; visible UI blockers override other signals


## 5. Known Limitations & Rough Edges

### From CHANGELOG.md & Issues

#### Fixed in Latest Releases (v0.8.0-0.8.2)
- **Windows arm64 installer** race condition in emulation
- **Chinese IME** commits now reach panes on macOS
- **WSL Git status** redundant security scans
- **Busy sessions** CPU regression from hidden pane wakeups
- **Session Navigator** now searches renamed labels
- **Elevated Windows panes** no longer show PowerShell admin decoration
- **Modal IME** cursor now anchors to text field correctly
- **Prefix disambiguation** for shifted punctuation on non-US keyboards

#### Known Remaining Challenges
(Inferred from codebase):

1. **Terminal Compatibility**:
   - Only works on xterm-256color compatible terminals
   - Kitty graphics require specific terminal support
   - Remote SSH requires specific terminal features on both ends
   - Windows ConPTY has special handling for multi-line operations

2. **Agent Detection Reliability**:
   - Pattern matching is agent-specific; new agents need manifests
   - Title spinner detection can miss edge cases (e.g., Unicode variations)
   - False positives possible when agent output matches pattern by accident
   - "Unknown" state common for non-standard tools

3. **Performance**:
   - Large scrollback (10MB default) impacts rendering on slow systems
   - High-rate background output can impact render cadence
   - Git status refresh on Windows requires process snapshots (CPU cost)

4. **Keyboard/Input**:
   - Keyboard enhancement modes vary by terminal
   - Shifted punctuation varies by keyboard layout
   - IME support is terminal-dependent (Ghostty, iTerm2, etc.)

5. **Cross-Platform**:
   - Windows support is "generally available" but still has edge cases
   - Preview Windows builds exist for endpoint protection scenarios
   - Remote client (Windows→Linux/macOS) is newer feature

#### Design Tradeoffs
- **Single Rust binary** (no Electron) means tighter integration but steeper build/distribution
- **Terminal-based UI** limits some visual polish but ensures SSH-ability
- **Agent detection via pattern matching** is lightweight but not foolproof
- **Local socket IPC** is fast but limits network transport (remote feature adds that)

### Not Implemented / Scope Limits
- No built-in terminal emulator window (relies on host terminal)
- No GUI version (TUI-only by design)
- No automatic fallback detection (user must configure agent types)
- No built-in code editor (agents launch their own)
- No web UI (remote requires CLI + local terminal)


## 6. Technical Specifics

### File Structure Summary

| Path | Purpose |
|------|---------|
| `src/main.rs` | Entry point, terminal setup |
| `src/app/` | Application state & business logic |
| `src/pane.rs` | Pane lifecycle, PTY interaction |
| `src/session.rs` | Session naming, env var handling |
| `src/workspace.rs` | Workspace layout, tabs, panes |
| `src/layout.rs` | BSP tree layout engine |
| `src/ui.rs`, `src/ui/` | Ratatui rendering |
| `src/detect/` | Agent state detection logic |
| `src/pty/` | PTY actor system, I/O management |
| `src/api/` | IPC server, request handler |
| `src/config/` | Config parsing, keybindings |
| `src/input/` | Terminal key parsing |
| `src/server/` | Server/daemon logic |
| `src/client/` | Client-side (CLI) logic |
| `src/integration/` | Integration hooks for Claude Code, etc. |
| `src/remote/` | Remote client support |
| `skills/herdr/` | Claude Code skill for herdr control |

### Command Line Examples

```bash
# Session management
herdr                               # Launch/attach default session
herdr session attach my-work        # Attach named session
herdr session list                  # List all sessions
herdr --session=project-x           # Specify session

# Agent control
herdr agent list                    # Show all agents
herdr agent start pi                # Start pi agent in available pane
herdr agent prompt claude "fix this" # Send text to agent
herdr agent send-keys claude "Enter"

# Pane management
herdr pane list                     # Show all panes
herdr pane split w1:t1:p1 --down    # Split pane vertically
herdr pane read w1:t1:p1            # Read pane output
herdr pane wait-output w1:t1:p1     # Wait for new output

# Workspace/Tab control
herdr workspace create              # Create workspace
herdr workspace focus w1            # Focus workspace
herdr tab create --workspace w1     # Create tab
herdr tab list --workspace w1       # List tabs
```

### Socket API Schema
- Request/response via bincode serialization
- Streaming support for long-running operations (pane read, wait-output)
- Subscriptions for state changes
- Methods defined in `src/api/schema.rs`

### Persistence Format
- **Session state**: JSONL snapshots (workspace/tab/pane layout)
- **Config**: TOML (toml crate with serde)
- **Agent sessions**: Serialized refs (session_ref_from_snapshot in agent_resume.rs)

---

**Report generated**: 2026-08-30
**Herdr version researched**: v0.8.2
**Repository**: https://github.com/herdrdev/herdr
