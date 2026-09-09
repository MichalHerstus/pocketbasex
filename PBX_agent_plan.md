# PBX Agent Enhancement Plan (Desktop Only)

## Overview
Six phases, each shippable independently. Uses `marked.min.js` for markdown. Mobile templates untouched.

## Status: ALL PHASES COMPLETE ✅

| Phase | Title | Status |
|-------|-------|--------|
| 1 | Streaming Markdown + Regenerate | ✅ Complete |
| 2 | Interactive Chat Responses | ✅ Complete |
| 3 | Conversation Persistence | ✅ Complete |
| 4 | More Agent Tools | ✅ Complete |
| 5 | Suggested Prompts & Slash Commands | ✅ Complete |
| 6 | Structured Form-Fill | ✅ Complete |

## Markdown Library
`marked.min.js` (~40KB) added to `views/assets/`, referenced in `agent.html`.

---

## Phase 1: ✅ COMPLETE Streaming Markdown + Regenerate
**Files**: `views/agent.html`, `views/assets/marked.min.js`
- Add `marked.min.js` to assets
- Replace `md()` with `marked.parse()` (esc before to prevent injection)
- Add Regenerate button (↻) on last assistant message
- CSS for headings, code blocks, blockquotes, regenerate button

## Phase 2: ✅ COMPLETE Interactive Chat Responses
**Files**: `pbai/render.go`, `pbai/tools.go`, `pbai/agent.go`, `main.go`, `views/agent.html`
- 2a: Clickable record links in tables + detail cards (render.go)
- 2b: Copy-to-clipboard on tables and code blocks (agent.html)
- 2c: `navigate_to` tool — agent suggests navigation (tools.go, agent.go, agent.html)
- 2d: Delete button on detail cards with confirm flow (render.go, agent.go, main.go, agent.html)

## Phase 3: ✅ COMPLETE Conversation Persistence
**Files**: `pb_migrations/`, `main.go`, `routes.go`, `views/agent.html`
- New `_conversations` PB collection (migration)
- Backend CRUD API (list, get, delete, rename)
- Auto-save after each turn, restore on page load
- Collapsible left sidebar with conversation list

## Phase 4: ✅ COMPLETE More Agent Tools
**Files**: `pbai/tools.go`, `pbai/agent.go`
- `query_related` (read): cross-collection relation queries
- `create_records_batch` (write, confirmed): insert up to 200 records
- `get_stats` (read): aggregate stats (count/min/max/avg/distinct)
- `run_action` (write, confirmed, superuser): execute custom actions
- `export_data` (read): CSV/JSON export with download URL

## Phase 5: ✅ COMPLETE Suggested Prompts & Slash Commands
**Files**: `main.go`, `routes.go`, `views/agent.html`
- `GET /api/ai/suggestions` — top 5 collections with record counts
- Welcome screen with suggestion chips when chat is empty
- Slash command palette (`/list`, `/create`, `/search`, `/schema`, `/stats`, `/help`)

## Phase 6: ✅ COMPLETE Structured Form-Fill
**Files**: `pbai/agent.go`, `views/form.html`, `views/agent.html`
- Add `FormFill` field to `ChatResult`
- Server-side extraction of `{"formFill":...}` from assistant text
- Simplified `applyAIResult()` with structured path + fallback
- Validation feedback (reportValidity + field flash animation)

## Implementation Order
```
Phase 1 (Markdown + Regen)       ← no backend deps
Phase 4 (More Tools)             ← parallel with Phase 1
Phase 6 (Form-Fill)              ← parallel with Phase 1
Phase 2 (Interactive Responses)  ← after Phase 1
Phase 3 (Persistence)            ← after Phase 2
Phase 5 (Onboarding)             ← after Phase 4
```

## Scope
- **In**: Desktop `/ai`, `/tabular/*` embedded, `/form/*` embedded
- **Out**: Mobile templates, Node.js, npm, build step, new UI frameworks
- **Testing**: `go build ./... && go vet ./... && go test ./...` after each phase
