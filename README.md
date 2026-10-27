# CritHook

CritHook is a lightweight, self-hosted bridge from GitHub and GitLab comments to configured review agents and back to the original discussion.

Implementation is in progress. The shared review-command parsing and the normalized task-input types described below exist today; webhook ingestion, task execution, and result publication are tracked in [the CritHook Project](https://github.com/users/zhuyihapi/projects/4).

## Review commands

A review is requested with a comment that starts with the `/review` command token:

```text
/review [--agent <name>] [focus...]
```

- The comment must start with `/review` (case sensitive) followed by whitespace or nothing. Anything else is not a command.
- `--agent <name>` or `--agent=<name>` selects an executor. The name must match one configured on the server, exactly and case sensitively; otherwise the command is rejected without creating a task. Omitting `--agent` selects the server's default executor. Options must appear before any focus text; `--agent` may be given only once and unrecognized options are rejected.
- Everything after the options is free-text review focus, preserved verbatim. Comment content is data only: it is never executed and can never select commands or credentials.

The normalized input for an accepted request (`internal/review.TaskInput`) covers GitHub issue and pull-request comments and GitLab issue and merge-request comments, distinguishes issue requirement reviews from code reviews, and carries a provider-scoped `SourceIdentity` that stays stable across platform redeliveries.

## Development

- Go 1.27, module path `github.com/zhuyihapi/CritHook`, standard library only.
- Checks: `gofmt -l .`, `go vet ./...`, `go test ./...`, `go build ./...`.
