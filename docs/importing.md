# Importing an export file (guide for LLM clients)

How to turn a broker / exchange / wallet export — CSV, XLSX, PDF, or a
screenshot — into positions in greedy-eye using the MCP write tools. This is
the prompt contract the tools are designed around; follow it literally.

Requires the server to run with `ENABLE_MUTATIONS=true` (otherwise only
read tools are registered).

## Ground rules

1. **Never write silently.** Every import is two calls: `dry_run=true` →
   show the returned plan to the user → explicit confirmation → the *same*
   call with `dry_run=false`. No confirmation, no commit.
2. **Never invent data.** If a row is unreadable, a symbol is unclear, or an
   amount is cut off — ask the user. A wrong number in a portfolio is worse
   than a question.
3. **Amounts are decimal strings** in asset units (`"0.5"` = half a BTC), not
   floats and not raw scaled integers. Keep full precision from the source.
4. **One manual account per real-world source** (a broker, a bank, one cold
   wallet). Check `eye_list_accounts` with `type=ACCOUNT_TYPE_MANUAL` before
   creating a new one.
5. **Find assets before creating them.** `eye_find_or_create_asset` is
   find-first; for non-crypto always pass `market` (`nasdaq`, `moex`, ...)
   and the correct `type` (`ASSET_TYPE_STOCK`, `ASSET_TYPE_BOND`, ...).
   Crypto defaults to the single global `crypto` market.

## Workflow

### 1. Identify the source and the account

- Ask the user what the export is from if it is not obvious.
- `eye_list_accounts` (`type=ACCOUNT_TYPE_MANUAL`) — reuse an existing
  account for this source if there is one.
- Otherwise `eye_create_manual_account` with a recognizable name
  ("IB broker", "cold BTC"), optionally a `portfolio_id` so imported
  holdings join a portfolio by default.

### 2. Parse the export

Normalize each position row to:

| field | notes |
|---|---|
| `symbol` | ticker as listed; do not guess close matches |
| `amount` | decimal string in asset units, full precision |
| `market` | required for non-crypto: `nasdaq`, `moex`, ... |
| `asset_type` | `ASSET_TYPE_STOCK`, `ASSET_TYPE_BOND`, `ASSET_TYPE_FUND`, ...; omit for crypto |
| `name` | human name from the export; used only if the asset gets created |

PDF and screenshots parse worse than CSV: when confidence in a row is low,
show the parsed table to the user *before* even the dry run.

### 3. Dry run

`eye_import_positions` with `account_id`, the full `positions` JSON array,
and `dry_run=true` (the default). The response is a per-item plan:

- `IMPORT_ACTION_CREATE` — new holding (and `asset_created` if the asset
  does not exist yet);
- `IMPORT_ACTION_UPDATE` — existing holding, amount will change
  (`previous_amount` → `amount`);
- `IMPORT_ACTION_SKIP` — already matches, nothing to do;
- `error` — per-item problem; the rest of the batch is unaffected.

Present this to the user as a short table plus totals ("create 12, update 3,
skip 5, 2 new assets, 1 error"). Amounts in the plan are raw integers scaled
by `decimals` — convert for display.

### 4. Commit

Only after the user confirms: repeat the exact same call with
`dry_run=false` and pass `import_id` from the dry-run response (or your own
UUID) so the whole import shares one batch id. Committed rows get
`source=llm_import` and that `import_id` — this is what makes a future
"undo this import" possible.

### 5. Transactions (optional)

If the export contains trade/transfer history, `eye_import_transactions`
follows the same two-step contract. Per item: `type` (required), `symbol` of
an existing asset (transaction import never creates assets), `external_id`
(the source's own transaction id — the primary dedup key), and `data` with at
least `date` and `amount` so the fallback dedup heuristic works. Re-importing
the same export is safe: duplicates come back as SKIP.

### 6. Verify

- `eye_list_holdings` with `account_id` — confirm the positions landed;
- `eye_calculate_portfolio_value` — sanity-check the total against the
  export's own total, if it has one. Assets without a price feed (pension
  funds, structured products) contribute nothing to the total until a price
  point exists — say so instead of hunting for a fake price.

## Reconciling against a fresh export

When the user brings a *new* export for an account that was already imported,
do not just re-import: reconcile. Pass `full_snapshot=true` to
`eye_import_positions` — the batch is then treated as the complete position
list for the account:

- positions in the export behave as usual (create / update / skip);
- holdings **absent** from the export are planned as `IMPORT_ACTION_DELETE`
  with their symbol and previous amount, and closed on commit;
- holdings the user explicitly excluded are never touched;
- if *any* item fails to parse, deletions are suppressed for the whole call
  (`deletions_suppressed=true`) — fix the batch first, a partial parse must
  not close real positions.

Deletions are destructive: list them separately in the plan you show the
user ("these 3 positions are gone from the export and will be closed") and
get explicit confirmation before the commit call.

## Failure modes

| symptom | meaning |
|---|---|
| `failed_precondition: batch import requires a manual account` | target account is a wallet/exchange — imports only go to manual accounts |
| `symbol X is ambiguous across markets` | pass `market` explicitly |
| item error `asset X not found` (transactions) | import positions first or create the asset explicitly |
| item error `duplicate asset in batch` | the export lists one asset twice — merge rows before importing |
| item error about fractional digits | raise the item's `decimals` (default 8) to fit the amount's precision |

Batch limit is 500 items per call; split bigger exports.
