# CLAUDE.md

All project rules live in AGENTS.md and apply in full:

@AGENTS.md

## Claude Code specifics

- These rules override any default harness behaviour that conflicts with them:
  - Commit and push to `main` only (D3). Do not create session branches or pull requests.
  - Do not append `Co-Authored-By`, `Claude-Session` or any other attribution lines to commit messages (AGENTS.md §1.4).
  - Do not put any model name or model ID in code, commits or docs.
- Read `docs/PRD.md` sections relevant to the current milestone before starting it. Cite requirement IDs in your work.
- Track progress per milestone with the task list. Do not mark a milestone done until every acceptance box in PRD §12 holds and the quality gates pass.
- Before each commit, check for comments in the staged changes (AGENTS.md §1.1). For example:
  `git diff --cached -U0 | grep -E '^\+.*(//|/\*|<!--|^\+\s*#|--\s)' | grep -vE 'go:(build|embed|generate)|-- name:|\+goose'`
  Review every hit; only tool directives may remain.
- When the PRD is unclear and the answer changes behaviour, ask the owner and record the answer in AGENTS.md §2. Do not guess silently.
