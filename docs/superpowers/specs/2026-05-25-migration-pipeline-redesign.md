# Migration Pipeline Redesign

## Goal

Replace the current migration implementation with a clean, durable pipeline model. The new system uses a wizard-style frontend backed by persistent migration plans, jobs, items, conflicts, and events.

The first implementation will support Nexus as the only source plugin, while the architecture must allow additional sources later.

## Scope

In scope:

- Repository configuration migration
- Repository group/virtual membership migration
- Nexus privileges, roles, users, and user-role relations
- Artifact/component scanning and binary migration
- Conflict precheck and user-selected conflict policies
- Durable progress, event logs, retry, pause/resume, cancel, and restart recovery
- New frontend migration wizard

Out of scope for the first implementation:

- Scheduled recurring sync
- Non-Nexus source plugins
- Automatic overwrite of existing repositories, users, or permissions
- Compatibility with old `migration_tasks` and `migration_items` logic

The old migration model is deprecated. New code should not read or write old migration tables.

## Architecture

The new migration system is centered on `MigrationPlan`.

```text
MigrationPlan
  ├── Scan Stage
  ├── Precheck Stage
  ├── Execute Stage
  └── Verify Stage

Each stage owns MigrationJobs.
Jobs may own MigrationItems.
All meaningful status changes produce MigrationEvents.
Precheck produces MigrationConflicts.
```

### Core Layers

#### Source Plugin Layer

`MigrationSource` hides source-specific behavior.

The first implementation is `NexusSource`. It handles Nexus REST API paths, repository detail fallback behavior, continuation tokens, role/privilege parsing, user listing, component listing, and asset downloads.

No planner, prechecker, executor, handler, or frontend API should call Nexus REST details directly.

#### Planner Layer

Planner scans the selected source scope and writes stable migration jobs/items. It does not create repositories, users, roles, blobs, or artifacts in the target system.

Planner output includes jobs for:

- repository config
- group membership
- permissions
- roles
- users
- artifact scanning
- artifact copying

#### Precheck Layer

Precheck reads the planned jobs/items and compares them with local state. It writes `migration_conflicts` and does not modify target data.

Conflicts include:

- repository already exists
- user email already exists
- role already exists
- group member is missing
- target repository is missing
- storage backend is missing
- Nexus repository detail is unavailable
- artifact target path conflict

#### Executor Layer

Executors perform target-side changes. Execution is split by job kind:

- `RepoConfigExecutor`
- `GroupMembershipExecutor`
- `PermissionExecutor`
- `RoleExecutor`
- `UserExecutor`
- `ArtifactExecutor`

Executors must be idempotent and must follow conflict policies chosen during precheck.

#### Scheduler and Recovery Layer

Scheduler owns queueing, stage progression, concurrency, retry, pause/resume, cancel, and service-start recovery.

Recovery reads persisted database state. It must not rely on in-memory maps for correctness.

#### Event and Progress Layer

All user-visible logs and progress updates are stored in `migration_events`. The frontend reads plan/job/item/event APIs instead of process memory.

## Data Model

### `migration_plans`

Represents one migration plan.

Key fields:

```text
id
name
source_type
source_url
password_encrypted
source_auth_json
status
current_stage
selected_scope_json
conflict_policy_json
stats_json
created_at
updated_at
started_at
completed_at
```

Recommended statuses:

```text
draft
scanning
prechecking
precheck_failed
ready
running
paused
verifying
completed
failed
cancelling
cancelled
```

Recommended stages:

```text
scan
precheck
execute
verify
done
```

### `migration_jobs`

Represents an executable unit under a plan.

Key fields:

```text
id
plan_id
kind
status
source_key
target_key
depends_on_json
attempt_count
max_attempts
checkpoint_json
error_code
error_message
created_at
updated_at
started_at
completed_at
```

Job kinds:

```text
repo_config
group_membership
permission
role
user
artifact_scan
artifact_copy
verify
```

Job statuses:

```text
pending
blocked
running
completed
failed
skipped
cancelled
```

### `migration_items`

Represents high-volume details, mainly artifacts/assets.

Key fields:

```text
id
plan_id
job_id
kind
source_repository
source_id
source_path
source_format
source_name
source_version
target_repository
target_path
status
checksum_json
size_bytes
attempt_count
error_code
error_message
checkpoint_json
created_at
updated_at
completed_at
```

Item kinds:

```text
artifact
asset
```

Item statuses:

```text
pending
running
completed
failed
skipped
```

### `migration_conflicts`

Stores precheck conflicts and selected policies.

Key fields:

```text
id
plan_id
kind
severity
source_key
target_key
message
suggested_policy
selected_policy
payload_json
resolved_at
created_at
```

Conflict severities:

```text
warning
blocking
```

Conflict kinds:

```text
repo_exists
email_exists
role_exists
missing_group_member
source_detail_unavailable
target_repository_missing
storage_backend_missing
artifact_target_conflict
```

### `migration_events`

Stores progress and audit events.

Key fields:

```text
id
plan_id
job_id nullable
item_id nullable
level
event_type
message
payload_json
created_at
```

Levels:

```text
info
warn
error
```

Event types:

```text
status_changed
conflict_found
policy_applied
retry_scheduled
item_completed
source_warning
job_failed
plan_recovered
```

## State Machine

Normal flow:

```text
draft
  → scanning
  → prechecking
  → ready
  → running
  → verifying
  → completed
```

Exceptional flow:

```text
any stage → failed
any stage → cancelling → cancelled
prechecking → precheck_failed
running → paused
paused → running
```

### Draft

Stores source information, selected scope, execution parameters, and target mapping strategy. It performs no target writes.

### Scanning

Calls `MigrationSource` and writes migration jobs/items/events only.

Repository detail failures such as Nexus 405 are recorded as source warning events during scanning. Precheck later converts them into conflicts only when the missing detail affects a selected migration action and no safe fallback exists.

### Prechecking

Creates conflicts from source and target comparison. Blocking conflicts move the plan to `precheck_failed`. After the user applies policies, the plan becomes `ready`.

### Running

Executes jobs in dependency order. A simple first implementation can run config/security jobs first, artifact jobs second.

Each job and item must persist status before external side effects and write terminal status afterward.

### Verifying

First implementation performs lightweight checks:

- repository count and mapping consistency
- role/user relation consistency
- artifact item totals
- migrated asset checksum/size availability
- group membership existence

Verification failure leaves the plan failed at `verify` stage.

## Source Interface

```go
type MigrationSource interface {
    TestConnection(ctx context.Context) error
    ListRepositories(ctx context.Context) ([]SourceRepository, error)
    GetRepositoryDetail(ctx context.Context, name string) (*SourceRepositoryDetail, error)
    ListRoles(ctx context.Context) ([]SourceRole, error)
    ListPrivileges(ctx context.Context) ([]SourcePrivilege, error)
    ListUsers(ctx context.Context) ([]SourceUser, error)
    ListComponentsPage(ctx context.Context, repo string, cursor string) (SourceComponentPage, error)
    DownloadAsset(ctx context.Context, assetURL string) (AssetStream, error)
}
```

`NexusSource` is the only first implementation.

## Backend Package Layout

```text
internal/migration/
  domain/
  source/
    nexus/
  planner/
  precheck/
  executor/
  scheduler/
  repository/
  service/
```

### `domain`

Defines plan/job/item/conflict/event models, statuses, stage constants, error codes, and policy constants.

### `source/nexus`

Contains Nexus API client and conversion from Nexus-specific DTOs to source-domain DTOs.

### `planner`

Builds the job graph and high-volume items from selected scope.

### `precheck`

Detects conflicts and applies selected policies.

### `executor`

Executes idempotent job handlers.

### `scheduler`

Runs plans, advances stages, enforces concurrency, retries failed work, pauses/resumes/cancels plans, and recovers interrupted plans.

### `repository`

Contains data access for new migration tables.

### `service`

Application service used by HTTP handlers.

## HTTP API

```text
POST   /api/v1/migration/sources/test
POST   /api/v1/migration/plans
GET    /api/v1/migration/plans
GET    /api/v1/migration/plans/:id

POST   /api/v1/migration/plans/:id/scan
POST   /api/v1/migration/plans/:id/precheck
POST   /api/v1/migration/plans/:id/conflicts/apply
POST   /api/v1/migration/plans/:id/start
POST   /api/v1/migration/plans/:id/pause
POST   /api/v1/migration/plans/:id/resume
POST   /api/v1/migration/plans/:id/cancel
POST   /api/v1/migration/plans/:id/retry

GET    /api/v1/migration/plans/:id/jobs
GET    /api/v1/migration/plans/:id/items
GET    /api/v1/migration/plans/:id/conflicts
GET    /api/v1/migration/plans/:id/events
```

Old migration endpoints should be removed from the frontend path. If routes must remain temporarily for backend compatibility, they should return `410 Gone`. New frontend code should use only the new API.

## Frontend UX

Replace the current split migration page with a unified wizard.

Suggested files:

```text
web/src/views/MigrationWizardPage.vue
web/src/components/migration/SourceConnectionStep.vue
web/src/components/migration/ScopeSelectionStep.vue
web/src/components/migration/ScanProgressStep.vue
web/src/components/migration/ConflictResolutionStep.vue
web/src/components/migration/ExecutionProgressStep.vue
web/src/components/migration/MigrationResultStep.vue
web/src/api/migrationV2.ts
```

### Step 1: Connect Source

Fields:

```text
source type: Nexus
URL
username
password
```

Actions:

- test connection
- create or update draft plan

### Step 2: Select Scope

User selects:

```text
repository configuration
  hosted/local repositories
  proxy repositories
  group/virtual repositories
  group memberships

security configuration
  privileges
  roles
  users
  user-role relations

artifact data
  source repositories
  target strategy
```

Target strategies:

- keep source repository structure
- map to selected target repository

### Step 3: Scan

Shows discovered counts and source warnings. Completion automatically enables precheck.

### Step 4: Resolve Conflicts

Displays blocking conflicts and warnings grouped by kind. User chooses policies such as skip, map existing, rename, use fallback, create missing, or fail.

### Step 5: Execute

Shows total progress, config/security progress, artifact progress, failed jobs/items, and event log.

Actions:

- pause
- resume
- cancel
- retry failed work

### Step 6: Result

Shows success, partial success, or failure, with counts for repositories, memberships, permissions, roles, users, artifacts, skipped items, and failed items.

## Error Codes

Use explicit error codes instead of plain text-only errors.

Recommended first set:

```text
SOURCE_UNAVAILABLE
SOURCE_AUTH_FAILED
SOURCE_REPOSITORY_DETAIL_UNAVAILABLE
SOURCE_COMPONENT_PAGE_FAILED
TARGET_REPOSITORY_EXISTS
TARGET_EMAIL_EXISTS
TARGET_ROLE_EXISTS
TARGET_GROUP_MEMBER_MISSING
TARGET_STORAGE_BACKEND_MISSING
ARTIFACT_DOWNLOAD_FAILED
ARTIFACT_CHECKSUM_MISMATCH
ARTIFACT_STORAGE_FAILED
JOB_CANCELLED
JOB_RETRY_EXHAUSTED
```

## Conflict Policies

First implementation supports:

```text
skip
map_existing
rename
use_fallback
create_missing
fail
```

Do not implement overwrite in the first version. It can be added later with explicit user confirmation and audit logging.

## Idempotency Rules

### Repository Config

- Create if absent.
- Use existing if policy is `map_existing`.
- Rename if policy is `rename`.
- Re-running must not create duplicate repositories.

### Group Membership

- Use first-or-create semantics.
- Missing group or member follows selected policy.

### Permissions and Roles

- Permission identity is `(resource, action)`.
- Role identity is `name`.
- Role-permission links use upsert/first-or-create semantics.

### Users

- Username is unique.
- Email conflicts follow selected policy.
- User-role links use upsert/first-or-create semantics.

### Artifacts

- Blob storage reuses checksum/blob keys when available.
- Existing target artifact path follows selected policy.
- Retry only failed items.
- Completed items are never re-downloaded unless explicitly reset in future functionality.

## Recovery

On service startup, find plans in active states:

```text
scanning
prechecking
running
verifying
cancelling
```

First implementation should recover them to `paused` and write a `plan_recovered` event. The user resumes manually.

Running jobs and running items should be reset to `pending` or `failed` according to whether they had an external side effect checkpoint. The first implementation can reset them to `pending` if item execution is idempotent.

## Testing Strategy

### Unit Tests

- `NexusSource`
  - repository detail 405 fallback
  - continuation token pagination
  - user/role/privilege decoding
- `Planner`
  - selected scope to jobs/items
  - group repository dependencies
  - artifact scan checkpoints
- `Prechecker`
  - repository exists
  - email exists
  - role exists
  - missing group member
  - missing storage backend
- `Executor`
  - idempotent execution per job kind
  - policy behavior
  - error code persistence

### Integration Tests

- AutoMigrate creates all new migration tables.
- Create draft plan, scan, precheck, apply policies, execute, verify.
- Restart recovery moves active plans to paused.
- Failed item retry only retries failed items.

### Frontend Tests

- Wizard step transitions
- Scope selection and select-all behavior
- Conflict policy selection
- Pause/resume/cancel button states
- Event log refresh

### Manual Validation

Validate against a small Nexus instance containing:

- proxy repository detail returning 405
- group repository with members
- duplicate email user
- existing target repository
- a repository with a small artifact set

## Migration From Existing Code

Use a clean break.

- Do not migrate old task data.
- Stop using old `migration_tasks` and `migration_items` from new code.
- Prefer deleting old migration service/worker/handler code after V2 is wired.
- Old tables may remain in DB to avoid destructive deployment behavior, but they are not part of the new runtime.

## Implementation Phasing

### Phase 1: Backend foundation

Create new domain models, repositories, AutoMigrate entries, source interface, Nexus source, and basic app service.

### Phase 2: Planning and precheck

Implement scan, job/item creation, conflict generation, and conflict policy application.

### Phase 3: Execution and recovery

Implement scheduler, job executors, item executors, event logs, retry, pause/resume/cancel, and startup recovery.

### Phase 4: Frontend wizard

Build the unified migration wizard and switch routes/API calls to V2.

### Phase 5: Remove legacy runtime

Remove old handler routes and old runtime code once V2 is verified.
