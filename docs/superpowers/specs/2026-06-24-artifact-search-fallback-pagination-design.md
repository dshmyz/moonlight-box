# Artifact Search Fallback Pagination Design

## Goal

Make package search results complete when `PackageSearchService` must fall back to the `artifacts` table, without loading or silently truncating all matching artifact rows in Go memory.

## Scope

- Keep the existing `packages` table path as the default search path.
- Replace the pre-aggregation `LIMIT 50000` in `searchFromArtifacts`.
- Preserve SQLite and PostgreSQL support.
- Preserve the current API response shape and package-level pagination semantics.

## Non-goals

- Do not change the `packages` table update pipeline.
- Do not add an `incomplete` response mode; the fallback must return complete results.
- Do not change public query parameters or frontend behavior.

## Design

`searchFromArtifacts` will build one filtered artifact relation, then let the database aggregate it by `(repository_id, format, name)` before sorting and paginating. The service will execute two queries against the same relation:

1. Count grouped package rows for `SearchResult.Total`.
2. Select only the requested grouped page, ordered by the requested package sort.

The grouped query returns the existing fallback fields: a stable artifact ID, package identity, maximum update time, artifact-row version count, and representative license/description data. Repository names continue to be fetched only for the returned page.

The shared filtering and grouping SQL uses `GROUP BY`, aggregate functions, derived tables, `LIMIT`, and `OFFSET`, all supported by SQLite and PostgreSQL. Version pattern compilation remains the only database-dialect boundary: simple exact/`*`/`?` patterns use the existing portable `LIKE` path; patterns containing `[` continue to use the fallback's Go `filepath.Match` semantics until a separately designed dialect-specific matcher is added. To retain correctness for that pattern, the fallback will stream filtered rows in bounded batches and aggregate them fully rather than truncate the result set.

## Error Handling

Any database query error is returned unchanged to the existing handler. The fallback never returns a successful but partial package collection.

## Tests

- More than 50,000 matching artifacts for one package plus an older second package returns both packages and `Total == 2`.
- The same dataset preserves `updated_at` ordering and page boundaries.
- Existing exact, `*`, `?`, and `[` version-filter tests continue to pass on SQLite.
