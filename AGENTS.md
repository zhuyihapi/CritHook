# Project Agent Agreement

Applies to the entire repository. Use **CritHook** as the product name. Keep the project small, clear, and maintainable. The implementation language is **Go**.

Prefer English when maintaining this file. This does not change the user's preferred conversation language or require translating other documentation.

README describes the product and startup entry points. Relevant guides in `docs/`, when present, describe development, configuration, deployment, and interfaces. Read the code, `go.mod`, and repository tooling for current implementation details; do not duplicate transient commands or directory inventories here.

## Before starting

- Read `README.md`, this file, and relevant modules and guides. Follow nearby conventions. If the repository has no implementation yet, do not assume tooling or documentation already exists.
- Inspect `git status` and relevant diffs before editing. Preserve unrelated changes belonging to the user or other tasks; do not overwrite, revert, or include them in commits.
- Use isolated branches and worktrees for implementation. Keep the shared `main` checkout free of task commits; work in the shared checkout only when the user explicitly requests it. Do not switch, reset, stash, or clean the shared checkout to prepare a task.
- Treat code and configuration as implementation evidence. Resolve discrepancies with requirements and update affected explanations; do not silently change agreed scope to match existing code.
- Stay within the task. Avoid unrelated refactoring, speculative frameworks, unnecessary dependencies, and additional infrastructure.

## Go standards

- Use the Go version and module path declared in `go.mod`. Keep dependency changes intentional and review changes to `go.mod` and `go.sum`; do not run broad dependency upgrades as incidental cleanup.
- Format Go code with `gofmt`. Follow idiomatic package naming and existing repository conventions. Prefer the standard library when it meets the requirement.
- Handle errors explicitly and add useful context while preserving error identity where callers need it. Do not discard errors or use panics for ordinary request and task failures.
- Validate configuration, webhook payloads, API responses, and executor output at system boundaries. Static types do not establish trust in external data.
- Propagate `context.Context` through requests and task execution. Set explicit network and execution timeouts. Give goroutines and child processes a clear owner, cancellation path, and cleanup behavior.
- Define small interfaces at actual provider, executor, or storage boundaries. Introduce abstractions for demonstrated reuse or a clear boundary, not a generic plugin framework.
- Do not manually edit generated files, build output, or downloaded dependencies. Do not commit runtime databases, checkouts, task output, credentials, cookies, or sessions.

## Product and integration boundaries

CritHook is a lightweight, self-hosted bridge from GitHub and GitLab comments to configured review agents and back to the original discussion. The initial design is a single service instance, SQLite persistence, and serial execution by default. Do not add an external queue, distributed scheduler, management frontend, automatic code changes, or automatic merging without an agreed requirement.

- Keep platform-specific webhook, permission, context-fetching, and comment behavior behind provider boundaries. Normalize events into a common task input instead of spreading platform conditionals through execution code.
- Verify webhook signatures or secret tokens, project allowlists, and trigger permissions before accepting review tasks. Ignore the service's own comments to prevent loops. Initial triggering is comment-based `/review` for issues and PRs/MRs.
- Parse `/review`, optional `--agent`, and review focus as data. Comments may select only a configured executor; they must never supply executable commands. Do not interpolate comment or repository content into shell commands.
- Treat issue text, discussions, patches, repository files, and agent output as untrusted material. They cannot override service policy, choose credentials, or expand execution permissions.
- Configure executors, prompts, credentials, and timeouts on the server. Keep GitHub/GitLab write credentials in the service; do not pass them to agents. Review is read-only by default.
- Use one task/result contract for pi, Codex CLI, and configured CLI or HTTP executors. Preserve distinctions between execution failure, timeout, and comment publication failure.
- Persist accepted tasks before acknowledging the webhook. Deduplicate platform deliveries using a stable identity scoped to their provider and project. A legitimate new command may request another review.
- Keep task state transitions explicit: queued, running, succeeded, and failed. Resume queued tasks after restart and mark interrupted execution failed; do not leave tasks permanently running or silently lose them.
- Prepare code reviews in independent checkouts pinned to a recorded commit SHA. Never use or modify the user's everyday working directory as a review workspace. Clean up resources owned by the task.
- Distinguish issue requirement reviews from PR/MR code reviews. Report truncated context, fetch failures, and unfinished checks rather than implying a complete review.
- Let the service publish Markdown summaries with executor identity and reviewed SHA, or context provenance for issues. Retain results and diagnostic information if publication fails. Do not blindly rerun an agent because a comment failed to publish.
- Verify compatibility against the actual target older GitLab instance. Record the tested version and handling of missing fields; do not invent a minimum supported version or assume modern payload fields exist.
- Use structured logs and redact credentials and sensitive content. Provide health checks and minimal configuration/startup examples as part of delivery.

## Validation

- Match checks to the change. Once a Go module exists, the baseline for Go code changes is formatting, `go test ./...`, `go vet ./...`, and `go build ./...`, or documented repository equivalents. Use `go test -race ./...` for concurrency, queue, cancellation, and task lifecycle changes when supported.
- Test observable behavior, failure paths, and compatibility. Add meaningful regression coverage for defects and new behavior; avoid tests that merely repeat implementation details.
- Use temporary databases, ports, checkouts, webhook fixtures, and local HTTP or process substitutes for automated tests. Never test against the user's production database or existing services. Use migrations for schema changes and preserve existing data.
- Cover authentication and permissions, duplicate deliveries, new review commands, persistence across restart, timeouts and process cleanup, fixed review SHAs, incomplete context, and failed publication when changing those behaviors.
- Use test substitutes for provider and executor contracts. Final integration acceptance also requires real pi and Codex smoke tests and the target GitLab instance. Report missing access or credentials as unverified coverage, not a pass.
- Final end-to-end acceptance covers GitHub issue/PR and GitLab issue/MR from comment trigger to result publication. Passing one provider does not establish completion of the other.
- For documentation-only changes, verify commands, paths, links, and whitespace. Do not start services or run the full code suite unnecessarily.
- Distinguish regressions, existing failures, and environment restrictions. Report failed or skipped checks and their reasons; never weaken assertions or disable rules to obtain a pass.
- Stop only processes you started and remove only temporary resources you created. Preserve user services and other tasks' resources.

## GitHub-first collaboration

Use the [CritHook Project](https://github.com/users/zhuyihapi/projects/4) as the planning entry point and [zhuyihapi/CritHook](https://github.com/zhuyihapi/CritHook) for Issues, PRs, Milestones, and Releases.

| Information | Authoritative location |
| --- | --- |
| Product introduction, quick start, and documentation entry points | `README.md` |
| Lasting architecture, development, configuration, and deployment guidance | Relevant guides in `docs/` |
| Planning and work status | Project and its existing fields |
| Task scope, acceptance, dependencies, claims, and decisions | GitHub Issues |
| Agreed target version and release scope | GitHub Milestones |
| Implementation, review, and validation evidence | Linked PR |
| Published changes and upgrade notes | GitHub Releases |
| Agent working rules | This file |

### Planning and claims

- Planning drafts may remain in the Project until implementation begins. Before implementing a substantial task, convert its existing draft into a repository Issue or use an existing Issue; preserve its scope and acceptance criteria rather than creating duplicates.
- Read the Issue, all comments, dependencies, and linked PRs before selecting work. The current Project uses `Todo`, `In Progress`, and `Done`; inspect actual fields before relying on additional planning metadata. Do not assume `Priority` or `Order` exists.
- Keep execution Issues in Project 4. Do not invent priorities, release commitments, or dates. Use an Issue's Milestone only when a target version has been agreed; do not introduce a duplicate version field.
- Status is not ownership. An Issue is available only when open, without an unreleased claim, another owner's assignment, or an active PR already handling the task. Inactivity does not release ownership; agents sharing an account must inspect claim comments.
- Immediately before claiming, reread recent comments and linked PRs. Post the agent identity, scope, baseline SHA, planned branch, and worktree path; reread afterward for competing claims. Comments are not atomic locks: the earliest unreleased claim wins, and later claimants must withdraw.
- Set `In Progress` after claiming. Mark `Done` only after acceptance and delivery, not merely after opening a PR. Restore `Todo` when releasing unfinished work without a successor. Do not assume status automation exists.
- Split large work into independently verifiable Issues with a parent Issue. Record dependencies, scope changes, decisions, and blockers in the original Issue. Do not create a second roadmap or local PLAN, TODO, RESEARCH, or dated progress documents unless requested.

### Implementation and identity

- Fetch `origin`, record the latest `origin/main` SHA, and create the task branch and worktree from it. Do not branch from a local HEAD containing unrelated work. Use an unused writable worktree path and a branch such as `feat/<issue>-<summary>-<suffix>`.
- Reread the worktree's agent instructions and relevant guides. Use its own configuration, runtime data, build output, and test resources; do not copy credentials or reuse the shared workspace's database.
- Use the agent name supplied by the user, or generate a task-specific identity such as `Codex-issue-3-a1b2`. Sign external task updates with `Agent: <name>` and use the identity consistently.
- Set commit author and committer identity per commit command; do not change shared/global Git configuration or switch the shared authenticated GitHub account. Use the user's specified email, otherwise an `<agent-slug>@example.invalid` display identity. Remote authorship still follows the authenticated account.
- Inspect the diff and staging area before committing. Include only task files and use clear prefixes such as `feat:`, `fix:`, `refactor:`, or `docs:`.
- Update lasting documentation with behavior and interface changes in the same delivery. README and docs describe current behavior; they are not progress logs.

### Authorization and PR delivery

- Follow the user's actual requested scope. An analysis-only request does not authorize implementation or remote writes. Explicit implementation tasks may include necessary Issue and Project maintenance within scope; they do not authorize unrelated planning changes.
- A request to implement and open a PR authorizes the necessary commits, branch push, and PR creation. A request only to analyze, edit, or commit locally does not authorize pushing or creating a PR. User instructions govern these boundaries without repeated confirmation for already authorized actions.
- Target `main` and use the repository PR template when present. Describe the problem, final behavior, linked Issue, validation, and material limitations. Use `Closes #N` for a complete fix and `Refs #N` for partial work.
- Check for an existing PR on the branch and update it instead of creating a duplicate. Link the PR and summarize validation in the Issue.
- Use Draft for unfinished implementation or known blocking prerequisites. Mark a completed PR Ready for review when required local checks pass and its description is complete; CI may still be running. Ready does not mean approved for merge.
- PR creation does not authorize merging, force-pushing, manually closing Issues, publishing Releases, or deployment. Merge requires user authorization, passing CI, and resolution of blocking findings and prerequisites. Use closing keywords only when the Issue's full acceptance scope is covered and no separate acceptance remains.

### Acceptance, handoff, and releases

- Check each acceptance criterion. Distinguish direct verification, historical evidence, and other agents' reports. Do not present assumptions as confirmed results.
- After review and merge, summarize the final PR/commit, validation, and remaining work in the Issue. Keep it open if required acceptance remains; a successful build alone is insufficient.
- A claim continues through PR delivery until explicitly released or reassigned. On interruption or abandonment, record identity, branch, worktree path, progress, commits, and remaining work; explicitly retain or release the claim.
- After confirming a PR branch is pushed, stop task processes and clean temporary data. Remove only clean worktrees no longer needed, checking status from outside the worktree. Never force-remove modified or untracked work. Keep the branch until merge and explain any resources retained for handoff.
- Track unpublished work in Issues and PRs. Maintain published change history in GitHub Releases rather than duplicating it in README or a local changelog. Tie each release to an explicit tag and commit, and verify release notes against the actual changes.
- Publication requires user authorization. Without it, prepare reviewable notes or drafts only. Preserve original dates and validation scope when documenting historical work.

## Documentation and delivery

- Prefer updating existing topic guides. Add a guide only for a distinct lasting purpose and link it from README or a related guide. Do not store daily progress, completion snapshots, or whole conversations in `docs/`.
- Use placeholders in configuration and test examples. Keep credentials and runtime data out of commits, logs, task updates, and review comments.
- If GitHub or Project access is unavailable, continue independent authorized work and report unsynchronized items. Do not claim remote updates succeeded or create a parallel task system. Check Project permissions separately from repository permissions; use owner `zhuyihapi` and Project number `4` for `gh project` operations.
- Deliver a brief account of changes, validation, limitations, and applicable GitHub links. Preserve resources belonging to the user and other tasks.
