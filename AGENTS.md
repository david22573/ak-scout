# AGENTS.md

## AK Scout Model Memory

- Last validated live OpenRouter comparison was run on `2026-06-25`.
- Snapshot used for the comparison: `runs/scout/snapshots/snapshot_20260625_181247_BNBUSD_PERP.json`.
- Current `go run . decide` request path sends only:
  - raw `MarketSnapshot` JSON
  - derived `ScoutFeatures` JSON
  - a strict JSON schema response contract
- No journal history is injected into the model request today.

## Preferred Models For `go run . decide`

- Default for live AK Scout decision classification: `deepseek/deepseek-v4-pro`.
  - Reason: best cost/capability tradeoff among the tested models while still producing valid structured output with the current strict parser.
- Optional higher-cost cross-check: `openai/gpt-5.4`.
  - Reason: strongest usable `confidence_reason` and overall output quality in the 2026-06-25 live comparison, but materially more expensive than V4 Pro.

## Models To Avoid With Current Parser

- Avoid `qwen/qwen3-235b-a22b-thinking-2507` in the current pipeline unless parser/output handling is improved.
  - Observed failures on `2026-06-25`: one prefixed non-JSON response (`y.` before JSON), one timeout.
- Avoid `minimax/minimax-m1` in the current pipeline unless schema enforcement/output normalization is improved.
  - Observed failures on `2026-06-25`: repeated non-schema output with alternate field names and extra fields.

## Secrets Rule

- For one-off OpenRouter commands, source only `OPENROUTER_API_KEY` from `../ak-trader/.env`.
- Do not copy secrets into this repo, into snapshots, or into committed files.
