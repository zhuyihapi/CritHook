# CritHook

CritHook is a lightweight, self-hosted bridge from GitHub and GitLab comments to configured review agents and back to the original discussion.

Implementation is in progress. The shared review-command parsing, the normalized task-input types, and the SQLite task store described below exist today; webhook ingestion, task execution, and result publication are tracked in [the CritHook Project](https://github.com/users/zhuyihapi/projects/4).

## Review commands

A review is requested with a comment that starts with the `/review` command token:

```text
/review [--agent <name>] [focus...]
```

- The comment must start with `/review` (case sensitive) followed by whitespace or nothing. Anything else is not a command.
- `--agent <name>` or `--agent=<name>` selects an executor. The name must match one configured on the server, exactly and case sensitively; otherwise the command is rejected without creating a task. Configured executor names cannot contain whitespace. Omitting `--agent` selects the server's default executor. Options must appear before any focus text; `--agent` may be given only once and unrecognized options are rejected.
- Everything after the options is free-text review focus, preserved verbatim. Comment content is data only: it is never executed and can never select commands or credentials.

The normalized input for an accepted request (`internal/review.TaskInput`) covers GitHub issue and pull-request comments and GitLab issue and merge-request comments, distinguishes issue requirement reviews from code reviews, and carries a provider-scoped `SourceIdentity` that stays stable across platform redeliveries.

## Task persistence

Accepted requests are persisted as tasks in SQLite (`internal/store`). A task is created in the `queued` state and moves through an explicit lifecycle — `queued`, `running`, `succeeded`, `failed` — constrained by the schema; execution transitions are owned by the execution lifecycle built on the store.

Durable acceptance boundary: `store.AcceptTask` commits the task and its delivery record in one transaction, and a webhook may be acknowledged as accepted only after it returns without error. Duplicates are already durable from their first delivery and are acknowledged the same way. On error nothing was stored and the webhook must be answered as failed so the platform redelivers.

Deliveries are deduplicated on two stable identities, each scoped to provider and project: the platform delivery identifier, when present, and the triggering comment's `SourceIdentity`. Same names on another platform or project never collide. A redelivery returns the already accepted task; a different comment is a new command and is accepted again. The database uses WAL with `synchronous=FULL`, so acknowledged tasks survive a crash, and schema changes run through ordered migrations that preserve existing records.

## Development

- Go 1.27, module path `github.com/zhuyihapi/CritHook`. Standard library plus `modernc.org/sqlite` (pure-Go SQLite driver, no cgo) for task persistence.
- Checks: `gofmt -l .`, `go vet ./...`, `go test ./...`, `go build ./...`.
