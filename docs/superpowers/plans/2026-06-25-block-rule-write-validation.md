# Block Rule Write Validation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Validate every block-rule write consistently while retaining partial-success batch imports.

**Architecture:** `BlockRuleService.ValidateRule` is the single seam for rule invariants. Create and batch validate supplied rules; update loads the persisted rule, applies recognised changes to a copy, validates that complete state, and only then persists.

**Tech Stack:** Go 1.24, Gin, GORM, existing Go tests.

## Global Constraints

- Batch imports save valid rows and count invalid rows as `failed`.
- Allowed: `exact`/`wildcard`; `license` with `equals`/`contains`; `publish_time` with `before`/`after`/`within_last`.
- `before`/`after` require RFC3339; `within_last` requires an integer greater than zero.

---

### Task 1: Service validation seam

**Files:**
- Modify: `internal/service/block_rule_service.go`
- Modify: `internal/service/block_rule_service_test.go`

- [ ] **Step 1: Write failing tests**

Add table-driven tests for `ValidateRule`: accept unconditional, license-contains, publish-time-before, and publish-time-within-last; reject unknown match/type/op, stray condition fields, incomplete conditions, license with time operations, publish-time with license operations, invalid RFC3339, and zero/non-numeric `within_last`.

- [ ] **Step 2: Verify red**

Run: `go test ./internal/service -run '^TestValidateBlockRule$' -count=1`

Expected: FAIL because `ValidateRule` does not exist.

- [ ] **Step 3: Implement minimal validator**

Add `func ValidateRule(rule *model.BlockRule) error`; return descriptive errors for every failed invariant. Call it in `Create` before repository persistence and in `BatchCreate`, incrementing `failed` and continuing on validation failure.

- [ ] **Step 4: Verify green**

Run: `go test ./internal/service -run 'Test(ValidateBlockRule|BatchCreate)' -count=1`

Expected: PASS.

### Task 2: Validate merged updates and HTTP responses

**Files:**
- Modify: `internal/service/block_rule_service.go`
- Modify: `internal/api/http/block_rule_handler.go`
- Modify: `internal/service/block_rule_service_test.go`
- Modify: `internal/api/http/block_rule_handler_test.go`

- [ ] **Step 1: Write failing update and handler tests**

Assert changing an existing rule to an incompatible operation returns an error and leaves storage unchanged. Assert create/update HTTP requests with invalid rule fields return 400; assert a batch containing one valid and one invalid rule returns `success: 1`, `failed: 1`.

- [ ] **Step 2: Verify red**

Run: `go test ./internal/service ./internal/api/http -run 'Test(BlockRule.*Validation|BlockRuleBatchImport)' -count=1`

Expected: FAIL because `Update` forwards unvalidated maps and handlers map validation errors to 500.

- [ ] **Step 3: Implement merged validation**

In `Update`, fetch the current rule, apply only rule fields present in the update map to a copy, call `ValidateRule`, then call the repository update. In handlers, return `response.BadRequest` for service validation errors; remove duplicated condition validation from `Create` so all paths share the service seam.

- [ ] **Step 4: Verify green and regressions**

Run: `gofmt -w internal/service/block_rule_service.go internal/service/block_rule_service_test.go internal/api/http/block_rule_handler.go internal/api/http/block_rule_handler_test.go && go test ./internal/service ./internal/api/http -count=1`

Expected: PASS.

### Task 3: Full verification

- [ ] **Step 1: Run repository suite**

Run: `go test ./... && git diff --check`

Expected: PASS with no whitespace errors.
