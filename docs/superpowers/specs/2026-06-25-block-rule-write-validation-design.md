# Block Rule Write Validation Design

## Goal

Ensure every block-rule write path accepts only semantically valid rules while preserving partial-success batch imports.

## Design

`BlockRuleService` owns one deep validation module: `ValidateRule(*model.BlockRule) error`. Handlers remain transport adapters: they bind JSON, build a rule, call the service, and translate validation failures into HTTP 400 responses. No handler independently defines enum or condition compatibility rules.

The validator enforces non-empty package identity, legal match types, legal condition types and operations, and condition-field completeness. A rule without `ConditionType` must leave `ConditionOp` and `ConditionValue` empty. License rules allow only `equals` and `contains`, with a non-empty value. Publish-time rules allow only `before`, `after`, and `within_last`; the first two require RFC3339 timestamps and `within_last` requires a positive integer day count.

For updates, the service loads the stored rule, applies only recognised model fields from the update map to that copy, validates the resulting full rule, then persists the original requested fields. This validates cross-field changes such as clearing a condition value or changing a condition type without requiring clients to submit every field.

`BatchCreate` validates each rule before persisting it. Invalid rows increment `failed`; valid rows continue to be saved, preserving the current partial-success contract.

## Tests

- Service validation covers valid unconditional, license, and publish-time rules plus invalid enums, incomplete conditions, incompatible operator/type pairs, invalid timestamps, and non-positive `within_last` values.
- Batch creation saves valid entries and counts invalid entries as failed.
- Handler tests confirm invalid create and update payloads receive HTTP 400 rather than a persisted invalid rule.
