# Agent conversation memory (multi-turn context)

## Root cause (verified)

- `views/agent.html:218` sends only the current prompt: `messages: [{ role: 'user', content: text }]`.
- `pbai/agent.go:133-140` prepends the system prompt to whatever `req.Messages` arrives — completely stateless between requests.
- Result: follow-ups ("is correct", "add the record") reach the model with no prior context.

Backend already supports full history arrays — fix is frontend-only plus optional server clamp.

## Change 1 — conversation history in `views/agent.html`

- Add `let history = [];` module-level.
- `send()`:
  - request body becomes `messages: [...history.slice(-16), { role: 'user', content: text }]` (cap last 16 messages to bound prompt growth on a slow local model);
  - on success push to `history`:
    - `{ role: 'user', content: text }`;
    - normal answer: `{ role: 'assistant', content: data.finalText }` when non-empty;
    - fast-path table answer (records present, `finalText` empty): synthetic note so follow-ups keep context — `(fetched N records from collection X)` using `records[0].collectionName` if present, else `(fetched N records as a table)`;
    - pending action: `{ role: 'assistant', content: data.finalText }` ("Awaiting confirmation: …" summary).
- `confirmAction()` — after response, append an assistant turn so the next prompt knows what happened:
  - approved & ok: `'Executed: ' + data.message` (insert/update output text);
  - rejected: `'Action rejected by user.'`;
  - failed/expired: `'Action failed: ' + data.message`.
- Add a **New chat** button in the header: clears `history[]` + transcript DOM (fresh conversation without page reload).
- File attachments: only the newest turn can carry a file (unchanged behavior); older turns stay text-only.

## Change 2 — defensive clamp in `pbai/agent.go`

- In `Run()`, clamp incoming history to the last 40 messages before appending (`if len(history) > 40 { history = history[len(history)-40:] }`) — protects against oversized payloads regardless of client.

## Change 3 — test in `pbai/agent_test.go`

- `TestRunSendsHistoryToLLM`: mock server decodes the request body and asserts the messages array contains the seeded user+assistant turns followed by the new user turn (locks the passthrough contract incl. clamp behavior with >40 input messages).

## Verification

- `go build ./... && go vet ./... && go test ./pbai/`; rebuild `./pbx`.
- Manual E2E: "add record to produkty: XYZ, brambory, 200" → confirm mapping → Approve modal executes insert; then ask "how many records does produkty have now?" — agent must reference the earlier fetch/write. New chat resets context.

## Notes / tradeoffs

- Each turn resends growing history → slightly slower prompt processing; capped at 16 client-side / 40 server-side.
- No server-side sessions → nothing to expire, works across tabs independently.
- Streaming (Phase 14 candidate) unaffected.
