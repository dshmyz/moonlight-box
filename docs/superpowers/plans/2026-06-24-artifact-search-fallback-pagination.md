# Artifact Search Fallback Pagination Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Return complete, correctly paginated package search results when `PackageSearchService` falls back to artifact aggregation.

**Architecture:** Keep `packages` as the default search source. For exact, `*`, and `?` version filters, the database groups filtered artifacts before calculating total and selecting a page. For `[` glob patterns, preserve `filepath.Match` by scanning filtered rows in bounded keyset batches until exhaustion, then aggregate and paginate complete package groups.

**Tech Stack:** Go 1.24, GORM, SQLite, PostgreSQL, existing `PackageSearchService` tests.

## Global Constraints

- Preserve the current `SearchResult` JSON shape and package-level pagination semantics.
- Keep SQLite and PostgreSQL support.
- Do not change the `packages` update pipeline or public search parameters.
- Never return a successful but partial fallback result.

---

### Task 1: Capture the pre-aggregation truncation regression

**Files:**
- Modify: `internal/service/package_search_service_test.go`

**Interfaces:**
- Consumes: `NewPackageSearchService`, `Search`, and `SearchRequest`.
- Produces: regression coverage proving all matching artifacts participate in fallback package grouping.

- [ ] **Step 1: Write the failing test**

Add `TestSearchFromArtifactsDoesNotTruncateBeforeGrouping`. Migrate `Repository`, `Artifact`, and `ArtifactBlob` only; insert one npm repository, 50,000 recent `version` artifacts named `hot`, and one older `version` artifact named `old-but-valid`. Search using `{Type: "npm", Page: 1, PageSize: 20}` and assert both packages are present.

```go
if got.Total != 2 || got.RawCount != 50001 {
	t.Fatalf("total=%d raw=%d, want total=2 raw=50001", got.Total, got.RawCount)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/service -run '^TestSearchFromArtifactsDoesNotTruncateBeforeGrouping$' -count=1`

Expected: FAIL with `total=1 raw=50000`.

- [ ] **Step 3: Commit the regression test**

```bash
git add internal/service/package_search_service_test.go
git commit -m "test: cover complete artifact fallback aggregation"
```

### Task 2: Aggregate portable filters in the database

**Files:**
- Modify: `internal/service/package_search_service.go:257-446`
- Test: `internal/service/package_search_service_test.go`

**Interfaces:**
- Consumes: the existing artifact filter builder and `versionToSQLCondition`.
- Produces: a database-grouped fallback result for requests without `[` in `SearchRequest.Version`.

- [ ] **Step 1: Write a second failing pagination test**

Add `TestSearchFromArtifactsPagesGroupedPackagesAfterLargeArtifactSet`. Use the fallback-only SQLite setup with 50,000 `hot` artifacts and two older package names. Request page 2 with page size 1, sorted by `updated_at`; assert it returns the second package rather than an empty page.

```go
if got.Total != 3 || len(got.List) != 1 || got.List[0].Name != "older-one" {
	t.Fatalf("unexpected page: total=%d list=%#v", got.Total, got.List)
}
```

- [ ] **Step 2: Run both regression tests to verify they fail**

Run: `go test ./internal/service -run 'TestSearchFromArtifacts(DoesNotTruncateBeforeGrouping|PagesGroupedPackagesAfterLargeArtifactSet)$' -count=1`

Expected: FAIL because `LIMIT 50000` is still applied before grouping.

- [ ] **Step 3: Implement the grouped relation**

Extract current filters into a helper returning `whereClause` and arguments. For requests without `[`, use this relation both for count and page queries, retaining `searchableArtifactSQL("artifacts")`:

```sql
SELECT repository_id, format, name,
       MIN(id) AS id,
       COUNT(*) AS version_count,
       MAX(updated_at) AS updated_at
FROM artifacts
WHERE <existing filters>
GROUP BY repository_id, format, name
```

Run `SELECT COUNT(*) FROM (<grouped relation>) grouped` for `Total`, then sort the relation and apply `LIMIT ? OFFSET ?` for the requested page. Fetch representative attributes for returned IDs to preserve `license` and `description`.

- [ ] **Step 4: Run focused tests to verify they pass**

Run: `go test ./internal/service -run 'TestSearchFromArtifacts(DoesNotTruncateBeforeGrouping|PagesGroupedPackagesAfterLargeArtifactSet|VersionCharClassFallsBackToArtifactsPath)$' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the grouped fallback**

```bash
git add internal/service/package_search_service.go internal/service/package_search_service_test.go
git commit -m "fix: paginate grouped artifact fallback search"
```

### Task 3: Preserve complete `[` glob matching

**Files:**
- Modify: `internal/service/package_search_service.go`
- Test: `internal/service/package_search_service_test.go`

**Interfaces:**
- Consumes: `filepath.Match` and the existing artifact filters.
- Produces: complete bounded-memory aggregation for version patterns containing `[`.

- [ ] **Step 1: Write the failing character-class scale test**

Add `TestSearchFromArtifactsCharClassDoesNotTruncateBeforeMatching`. Create 50,000 recent npm artifacts with versions matching `[12].*`, plus an older matching package; search with `Version: "[12].*"` and assert both names and `Total == 2`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/service -run '^TestSearchFromArtifactsCharClassDoesNotTruncateBeforeMatching$' -count=1`

Expected: FAIL with the older package absent.

- [ ] **Step 3: Implement bounded keyset scanning**

For patterns containing `[`, query filtered artifacts in ascending ID batches of 1,000 using `id > ? ORDER BY id LIMIT ?`. Apply `filepath.Match` and update the current package accumulator for every row. Continue until the query returns fewer than 1,000 rows; do not use a global row cap.

```go
for lastID := uint(0); ; {
	var rows []rawArtifact
	if err := db.Raw(batchSQL, append(args, lastID, batchSize)...).Scan(&rows).Error; err != nil { return nil, err }
	if len(rows) == 0 { break }
	lastID = rows[len(rows)-1].ID
	// Match the version and aggregate every matching row.
	if len(rows) < batchSize { break }
}
```

- [ ] **Step 4: Run character-class and existing version-filter tests**

Run: `go test ./internal/service -run 'TestSearch(FromArtifactsCharClassDoesNotTruncateBeforeMatching|VersionCharClassFallsBackToArtifactsPath|FromPackagesWithVersionFilter)' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the complete character-class fallback**

```bash
git add internal/service/package_search_service.go internal/service/package_search_service_test.go
git commit -m "fix: complete character class artifact fallback search"
```

### Task 4: Verify database compatibility and the full suite

**Files:**
- Test: `internal/service/package_search_service_test.go`

**Interfaces:**
- Consumes: all changed search paths.
- Produces: final verification evidence.

- [ ] **Step 1: Format and run service tests**

Run: `gofmt -w internal/service/package_search_service.go internal/service/package_search_service_test.go && go test ./internal/service -count=1`

Expected: PASS.

- [ ] **Step 2: Run the full Go suite**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 3: Inspect the final diff**

Run: `git diff --check && git diff -- internal/service/package_search_service.go internal/service/package_search_service_test.go`

Expected: no whitespace errors and only intended search fallback changes.
