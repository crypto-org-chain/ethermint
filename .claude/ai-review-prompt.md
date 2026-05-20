# AI Daily Code Review — Prompt

## Setup

```sh
REPO=$(git rev-parse --show-toplevel) || { echo "ERROR: not in a git repo"; exit 1; }
cd $REPO || { echo "ERROR: cannot cd into $REPO"; exit 1; }
GHREPO=$(git -C $REPO remote get-url origin \
  | sed 's|.*github\.com[:/]\(.*\)\.git$|\1|;s|.*github\.com[:/]\(.*\)$|\1|') \
  || { echo "ERROR: cannot derive GitHub repo from git remote"; exit 1; }
```

## Step 1 — Read queue state

Read `$REPO/.claude/ai-review-queue.md` to find: `current_index`, `status`,
`issue_number`, `issue_url`. Store as `ISSUE_NUM` and `ISSUE_URL`.

- If `status` is `completed`, silently exit — do nothing.
- If `current_index >= 20` (queue exhausted): if `ISSUE_NUM` is set, post a
  final comment on the issue that all modules have been reviewed; then write
  `status: completed` to `$REPO/.claude/ai-review-queue.md` and stop.
- Otherwise, read the module path at `current_index`. Do NOT modify this file yet.

## Step 2 — Review the module

Review the code at that module path under `$REPO/`. Read the actual source
files carefully before forming any opinion.

If the path note says "root-level .go files only", read only the root `.go`
files listed and exclude subdirectories.

**WHAT TO FLAG** — only findings you can point to a specific line:
- Bugs: incorrect logic, wrong assumptions, off-by-one, nil dereference
- Race conditions: shared state accessed without locks, goroutine leaks
- Security: input not validated at trust boundaries, key material mishandled,
  injection vectors
- Correctness: invariants that can be violated, error return values silently
  ignored
- Performance: O(n²) in hot paths, unnecessary allocations in loops with
  evidence from the code

**WHAT NOT TO FLAG:**
- Style, naming, or formatting issues
- Theoretical risks that require multiple unlikely preconditions to trigger
- Defense-in-depth suggestions when a primary defense is already present
- Missing comments or documentation
- Refactoring suggestions unrelated to correctness or security
- Issues already guarded by the framework (e.g. Cosmos SDK invariant checks)
- Anything you cannot point to a specific file and line number

## Step 3 — Self-filter pass

Re-read each finding and ask: "Can I point to the exact line where this goes
wrong? Would a senior Go engineer agree this is a real issue, not a theoretical
one?" Drop any finding that fails this check.

## Step 4 — Write findings to temp file

```sh
BODY=$(mktemp /tmp/ai-review-body-XXXXXX.md)
```

Header: `## [AI Review] <module-name> — <date>`

Format each finding as:
```
**[Severity: HIGH/MEDIUM/LOW]** `file:line` — description and why it matters.
```

If no findings survive the self-filter, write:
`## [AI Review] <module> — <date>\nNo actionable issues found.`

## Step 5 — Publish findings (first run)

If `ISSUE_NUM` was `not created yet` when read in step 1:

```sh
ISSUE_URL=$(gh issue create --repo $GHREPO \
  --title "[AI Review] Code Quality Findings — ethermint" \
  --body-file $BODY)
```

If this fails, clean up (`rm $BODY`) and stop — do NOT proceed to step 7.

```sh
ISSUE_NUM=$(echo "$ISSUE_URL" | grep -oE '[0-9]+$')
```

Record both into the updated queue file (written in step 7, not here).

## Step 6 — Publish findings (subsequent runs)

Else (`ISSUE_NUM` was already set when read in step 1):

```sh
gh issue comment $ISSUE_NUM --repo $GHREPO --body-file $BODY
```

If this fails, clean up (`rm $BODY`) and stop — do NOT proceed to step 7.

## Step 7 — Update queue state

Only if step 5 or 6 succeeded:

```sh
rm $BODY
```

Write updated `ai-review-queue.md` directly to `$REPO/.claude/ai-review-queue.md`:
- `current_index` incremented by 1
- `last_reviewed_date` set to today
- reviewed module moved to Completed Reviews
- `issue_number` / `issue_url` filled in if this was the first run
