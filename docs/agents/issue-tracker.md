# Issue tracker: GitHub

Issues and PRDs for this repo live in GitHub Issues.

## Repository

`dshmyz/moonlight-box` on GitHub.

## Commands

- List issues: `gh issue list`
- Create issue: `gh issue create --title "..." --body "..."`
- View issue: `gh issue view <number>`
- Add comment: `gh issue comment <number> --body "..."`
- Close issue: `gh issue close <number>`
- List PRs: `gh pr list`
- View PR: `gh pr view <number>`

## External PRs as a triage surface

**No.** External PRs are not included in the triage queue. Only issues are triaged.

## When a skill says "publish to the issue tracker"

Use `gh issue create` with the appropriate title and body. Apply triage labels using `--label`.

## When a skill says "fetch the relevant ticket"

Use `gh issue view <number>` to read the full body and comments.