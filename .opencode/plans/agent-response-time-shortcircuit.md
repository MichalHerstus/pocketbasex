# Optimize agent response time — short-circuit after lone query_records

## Context (measured against local LM Studio, google/gemma-4-e4b)

"list all records in collection produkty" ≈ 17–20s total:

| Phase | Time | Notes |
|---|---|---|
| Call 1 (prompt 770 tok → tool call) | 5.7s | 90 hidden reasoning tokens |
| Call 2 (prompt 1190 tok → final answer) | 10.9s | generates a markdown table the UI discards |

User decisions: implement ONLY the short-circuit optimization now; streaming (SSE) planned separately later; slim-tool-result / prompt-compression / LM Studio tuning docs not selected.

## Change 1 — fast path in `pbai/agent.go` (`Run`)

Add a counter next to the existing capture var:

```go
transcript := []ChatMessage{}
var lastRecords []map[string]any
toolCallsSoFar := 0
```

Inside the per-message tool loop, increment before exec and return early after a successful first-and-only query_records:

```go
for _, tc := range msg.ToolCalls {
    ...
    toolCallsSoFar++
    resultText, err := tool.exec(a, args)
    if err != nil {
        resultText = "tool error: " + err.Error()
    } else if tool.name == "query_records" {
        lastRecords = nil
        _ = json.Unmarshal([]byte(resultText), &lastRecords)
        // tabular fast path: a lone successful query_records needs no
        // follow-up LLM pass - the UI renders the records table directly.
        if toolCallsSoFar == 1 && len(lastRecords) > 0 {
            return &ChatResult{Transcript: transcript, Records: lastRecords}, nil
        }
    }
    messages = append(messages, ...)
}
```

Semantics / guardrails:
- Fires only when query_records is the FIRST executed tool call of the turn AND it returned ≥1 record (`len(lastRecords) > 0`; "No accessible records found." leaves it nil → normal flow).
- Multi-step flows (get_collection_schema/list_collections first, or parallel calls making query_records the 2nd exec) keep the final LLM pass.
- FinalText stays "" — UI already renders table-only when `records` present; pending-action path untouched.

## Change 2 — update tests in `pbai/agent_test.go`

- `TestRunCapturesQueryRecords`: mock server counts requests; assert **exactly 1 LLM call** (no second round-trip), `res.Records` has the seeded product, `res.FinalText == ""`.
- New `TestQueryRecordsFastPathNotForMultiStep`: call 1 → get_collection_schema, call 2 → query_records, call 3 → final text; assert 3 LLM calls happened and final text passes through (guardrail holds).

## Change 3 — AGENTS.md

Extend the `/ai/chat` route line: note that when `query_records` is the only tool used and returns records, the response returns immediately without a follow-up LLM call.

## Verification

- `go build ./... && go vet ./... && go test ./pbai/`
- Rebuild `./pbx`, restart, ask "list all records in collection produkty" → expect ~5–6s (single LLM call), table-only answer.

## Deferred (user opted out for now)

- SSE token streaming (separate follow-up phase)
- Slimming LLM-facing tool results (truncate long values, drop meta keys)
- System-prompt/tool-description compression
- LM Studio tuning checklist (disable Reasoning toggle ≈3–8s/call, GPU offload, flash attention, minimal context)
