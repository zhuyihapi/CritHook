package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/zhuyihapi/CritHook/internal/review"
)

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tasks.sqlite")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store, path
}

func githubIssueInput(commentID, deliveryID string) review.TaskInput {
	return review.TaskInput{
		Project:    review.Project{Provider: review.ProviderGitHub, Owner: "octo", Repo: "app"},
		ReviewKind: review.ReviewIssue,
		Number:     42,
		CommentID:  commentID,
		Requester:  review.Requester{Login: "alice"},
		Command:    review.Command{Agent: "pi", Focus: "check error handling"},
		DeliveryID: deliveryID,
		ReceivedAt: time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC),
	}
}

func TestAcceptTaskCreatesQueuedTask(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	in := githubIssueInput("c1", "d1")
	task, outcome, err := store.AcceptTask(ctx, in)
	if err != nil {
		t.Fatalf("AcceptTask: %v", err)
	}
	if outcome != OutcomeAccepted {
		t.Fatalf("outcome = %q, want %q", outcome, OutcomeAccepted)
	}
	if task.State != StateQueued {
		t.Errorf("state = %q, want %q", task.State, StateQueued)
	}
	if task.ID == 0 {
		t.Error("task ID is zero")
	}
	if task.Input != in {
		t.Errorf("round-tripped input %+v, want %+v", task.Input, in)
	}
	if task.CreatedAt.IsZero() || task.UpdatedAt.IsZero() {
		t.Error("storage timestamps are zero")
	}

	got, err := store.TaskBySource(ctx, in.SourceIdentity())
	if err != nil {
		t.Fatalf("TaskBySource: %v", err)
	}
	if got != task {
		t.Errorf("TaskBySource = %+v, want %+v", got, task)
	}

	queued, err := store.TasksByState(ctx, StateQueued)
	if err != nil {
		t.Fatalf("TasksByState: %v", err)
	}
	if len(queued) != 1 || queued[0].ID != task.ID {
		t.Errorf("TasksByState(queued) = %+v, want the accepted task", queued)
	}
}

func TestAcceptTaskDuplicateDelivery(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	first, outcome, err := store.AcceptTask(ctx, githubIssueInput("c1", "d1"))
	if err != nil {
		t.Fatalf("first AcceptTask: %v", err)
	}
	if outcome != OutcomeAccepted {
		t.Fatalf("first outcome = %q, want %q", outcome, OutcomeAccepted)
	}

	again, outcome, err := store.AcceptTask(ctx, githubIssueInput("c1", "d1"))
	if err != nil {
		t.Fatalf("redelivered AcceptTask: %v", err)
	}
	if outcome != OutcomeDuplicateDelivery {
		t.Fatalf("redelivered outcome = %q, want %q", outcome, OutcomeDuplicateDelivery)
	}
	if again.ID != first.ID {
		t.Errorf("redelivery returned task %d, want the original task %d", again.ID, first.ID)
	}
	assertTaskCount(t, store, 1)
}

func TestAcceptTaskRedeliveryWithNewDeliveryID(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	first, _, err := store.AcceptTask(ctx, githubIssueInput("c1", "d1"))
	if err != nil {
		t.Fatalf("first AcceptTask: %v", err)
	}

	// Platforms may redeliver the same comment under a new delivery
	// identifier; the comment identity still deduplicates it.
	second, outcome, err := store.AcceptTask(ctx, githubIssueInput("c1", "d2"))
	if err != nil {
		t.Fatalf("second AcceptTask: %v", err)
	}
	if outcome != OutcomeDuplicateCommand {
		t.Fatalf("second outcome = %q, want %q", outcome, OutcomeDuplicateCommand)
	}
	if second.ID != first.ID {
		t.Errorf("new delivery ID created task %d, want the original task %d", second.ID, first.ID)
	}

	// The new delivery identifier was recorded, so its next delivery is a
	// duplicate delivery even without consulting the comment identity.
	third, outcome, err := store.AcceptTask(ctx, githubIssueInput("c1", "d2"))
	if err != nil {
		t.Fatalf("third AcceptTask: %v", err)
	}
	if outcome != OutcomeDuplicateDelivery {
		t.Errorf("third outcome = %q, want %q", outcome, OutcomeDuplicateDelivery)
	}
	if third.ID != first.ID {
		t.Errorf("third delivery returned task %d, want the original task %d", third.ID, first.ID)
	}
	assertTaskCount(t, store, 1)
}

func TestAcceptTaskWithoutDeliveryID(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	in := githubIssueInput("c1", "")
	if _, outcome, err := store.AcceptTask(ctx, in); err != nil || outcome != OutcomeAccepted {
		t.Fatalf("first AcceptTask = %q, %v; want %q, nil", outcome, err, OutcomeAccepted)
	}
	_, outcome, err := store.AcceptTask(ctx, in)
	if err != nil {
		t.Fatalf("second AcceptTask: %v", err)
	}
	if outcome != OutcomeDuplicateCommand {
		t.Errorf("second outcome = %q, want %q (dedup falls back to the comment identity)", outcome, OutcomeDuplicateCommand)
	}
	assertTaskCount(t, store, 1)
}

func TestAcceptTaskNewCommentIsNewCommand(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	if _, _, err := store.AcceptTask(ctx, githubIssueInput("c1", "d1")); err != nil {
		t.Fatalf("first AcceptTask: %v", err)
	}
	task, outcome, err := store.AcceptTask(ctx, githubIssueInput("c2", "d2"))
	if err != nil {
		t.Fatalf("second AcceptTask: %v", err)
	}
	if outcome != OutcomeAccepted {
		t.Fatalf("outcome = %q, want %q: a new comment is a new command even for the same issue", outcome, OutcomeAccepted)
	}
	if task.State != StateQueued {
		t.Errorf("state = %q, want %q", task.State, StateQueued)
	}
	assertTaskCount(t, store, 2)
}

func TestAcceptTaskIdentityScoping(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	if _, _, err := store.AcceptTask(ctx, githubIssueInput("c1", "d1")); err != nil {
		t.Fatalf("base AcceptTask: %v", err)
	}

	gitlabSameNames := githubIssueInput("c1", "d1")
	gitlabSameNames.Project.Provider = review.ProviderGitLab

	otherRepo := githubIssueInput("c1", "d1")
	otherRepo.Project.Repo = "other"

	tests := []struct {
		name string
		in   review.TaskInput
	}{
		{
			name: "same names on another provider",
			in:   gitlabSameNames,
		},
		{
			name: "same names in another project of the same provider",
			in:   otherRepo,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, outcome, err := store.AcceptTask(ctx, tt.in)
			if err != nil {
				t.Fatalf("AcceptTask: %v", err)
			}
			if outcome != OutcomeAccepted {
				t.Fatalf("outcome = %q, want %q", outcome, OutcomeAccepted)
			}
		})
	}

	// Both scoped variants were accepted alongside the GitHub octo/app task.
	assertTaskCount(t, store, 3)

	// The same delivery identifier is only compared within its provider and
	// project: reusing it in another project must deduplicate against that
	// project's own task, not against GitHub octo/app.
	dup := githubIssueInput("c1", "d1")
	dup.Project.Repo = "other"
	task, outcome, err := store.AcceptTask(ctx, dup)
	if err != nil {
		t.Fatalf("AcceptTask in other project: %v", err)
	}
	if outcome != OutcomeDuplicateDelivery {
		t.Fatalf("outcome = %q, want %q", outcome, OutcomeDuplicateDelivery)
	}
	if task.Input.Project.Repo != "other" {
		t.Errorf("deduplicated against task of repo %q, want other", task.Input.Project.Repo)
	}
	assertTaskCount(t, store, 3)
}

func TestAcceptTaskRejectsInvalidInput(t *testing.T) {
	store, _ := newTestStore(t)

	tests := []struct {
		name string
		mut  func(*review.TaskInput)
	}{
		{"unknown provider", func(in *review.TaskInput) { in.Project.Provider = "gitea" }},
		{"incomplete project", func(in *review.TaskInput) { in.Project.Repo = "" }},
		{"unsupported review kind", func(in *review.TaskInput) { in.ReviewKind = "wiki" }},
		{"non-positive number", func(in *review.TaskInput) { in.Number = 0 }},
		{"empty comment id", func(in *review.TaskInput) { in.CommentID = "" }},
		{"empty requester", func(in *review.TaskInput) { in.Requester.Login = "" }},
		{"unresolved agent", func(in *review.TaskInput) { in.Command.Agent = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := githubIssueInput("c1", "d1")
			tt.mut(&in)
			if _, _, err := store.AcceptTask(context.Background(), in); err == nil {
				t.Fatal("AcceptTask succeeded, want error")
			}
		})
	}
	assertTaskCount(t, store, 0)
}

func TestStoreFailurePreventsAcceptance(t *testing.T) {
	store, _ := newTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, _, err := store.AcceptTask(context.Background(), githubIssueInput("c1", "d1")); err == nil {
		t.Fatal("AcceptTask succeeded after Close, want error: a failed store must fail the webhook acknowledgement")
	}
}

func TestAcceptTaskCancelledContextStoresNothing(t *testing.T) {
	store, _ := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, _, err := store.AcceptTask(ctx, githubIssueInput("c1", "d1")); err == nil {
		t.Fatal("AcceptTask succeeded with cancelled context, want error")
	}

	// The transaction rolled back completely: neither the task nor its
	// delivery record may exist.
	assertTaskCount(t, store, 0)
	queued, err := store.TasksByState(context.Background(), StateQueued)
	if err != nil {
		t.Fatalf("TasksByState: %v", err)
	}
	if len(queued) != 0 {
		t.Errorf("TasksByState(queued) = %d tasks, want 0", len(queued))
	}
}

func TestTasksSurviveReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.sqlite")
	ctx := context.Background()

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	first, _, err := store.AcceptTask(ctx, githubIssueInput("c1", "d1"))
	if err != nil {
		store.Close()
		t.Fatalf("AcceptTask: %v", err)
	}
	gitlab := githubIssueInput("c2", "g1")
	gitlab.Project.Provider = review.ProviderGitLab
	gitlab.ReviewKind = review.ReviewCode
	gitlab.Command = review.Command{Agent: "codex", Focus: "review the diff"}
	if _, _, err := store.AcceptTask(ctx, gitlab); err != nil {
		store.Close()
		t.Fatalf("AcceptTask gitlab: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	loaded, err := reopened.TaskBySource(ctx, first.Input.SourceIdentity())
	if err != nil {
		t.Fatalf("TaskBySource after reopen: %v", err)
	}
	if loaded != first {
		t.Errorf("task after reopen = %+v, want %+v", loaded, first)
	}
	queued, err := reopened.TasksByState(ctx, StateQueued)
	if err != nil {
		t.Fatalf("TasksByState after reopen: %v", err)
	}
	if len(queued) != 2 {
		t.Fatalf("TasksByState(queued) after reopen = %d tasks, want 2", len(queued))
	}

	// Deduplication identities also survive the reopen.
	_, outcome, err := reopened.AcceptTask(ctx, githubIssueInput("c1", "d1"))
	if err != nil {
		t.Fatalf("AcceptTask after reopen: %v", err)
	}
	if outcome != OutcomeDuplicateDelivery {
		t.Errorf("outcome after reopen = %q, want %q", outcome, OutcomeDuplicateDelivery)
	}
}

func TestTaskBySourceMissing(t *testing.T) {
	store, _ := newTestStore(t)
	_, err := store.TaskBySource(context.Background(), "github/octo/app/issue/42/comment/missing")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("error = %v, want sql.ErrNoRows", err)
	}
}

func TestTasksByStateRejectsUnknownState(t *testing.T) {
	store, _ := newTestStore(t)
	if _, err := store.TasksByState(context.Background(), TaskState("paused")); err == nil {
		t.Fatal("TasksByState succeeded for unknown state, want error")
	}
}

func TestMigrationsPreserveRecords(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "tasks.sqlite")

	v1 := []migration{
		{version: 1, name: "create widgets", stmts: []string{
			`CREATE TABLE widgets (id INTEGER PRIMARY KEY, label TEXT NOT NULL)`,
		}},
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := migrate(ctx, db, v1); err != nil {
		db.Close()
		t.Fatalf("migrate v1: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO widgets (id, label) VALUES (7, 'kept')`); err != nil {
		db.Close()
		t.Fatalf("seed: %v", err)
	}
	db.Close()

	v2 := []migration{
		v1[0],
		{version: 2, name: "add widget notes", stmts: []string{
			`ALTER TABLE widgets ADD COLUMN note TEXT NOT NULL DEFAULT ''`,
		}},
	}
	db, err = sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db.Close()
	if err := migrate(ctx, db, v2); err != nil {
		t.Fatalf("migrate v2: %v", err)
	}

	var label, note string
	if err := db.QueryRowContext(ctx, `SELECT label, note FROM widgets WHERE id = 7`).Scan(&label, &note); err != nil {
		t.Fatalf("read preserved record: %v", err)
	}
	if label != "kept" || note != "" {
		t.Errorf("record after migration = (%q, %q), want (kept, \"\")", label, note)
	}

	var applied []int64
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan schema_migrations: %v", err)
		}
		applied = append(applied, v)
	}
	if len(applied) != 2 || applied[0] != 1 || applied[1] != 2 {
		t.Errorf("applied versions = %v, want [1 2]", applied)
	}

	// Re-running is a no-op.
	if err := migrate(ctx, db, v2); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
}

func TestMigrateRejectsBadLists(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		list []migration
	}{
		{
			name: "gapless from one",
			list: []migration{{version: 2, name: "second"}},
		},
		{
			name: "no duplicate versions",
			list: []migration{
				{version: 1, name: "first"},
				{version: 1, name: "first again"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(t.TempDir(), "t.sqlite")))
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer db.Close()
			if err := migrate(ctx, db, tt.list); err == nil {
				t.Fatal("migrate succeeded, want error")
			}
		})
	}
}

func TestMigrateRejectsDowngrade(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "t.sqlite")

	v2 := []migration{
		{version: 1, name: "first", stmts: []string{`CREATE TABLE t (id INTEGER PRIMARY KEY)`}},
		{version: 2, name: "second", stmts: []string{`CREATE TABLE u (id INTEGER PRIMARY KEY)`}},
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := migrate(ctx, db, v2); err != nil {
		db.Close()
		t.Fatalf("migrate v2: %v", err)
	}
	db.Close()

	db, err = sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db.Close()
	v1 := v2[:1]
	if err := migrate(ctx, db, v1); err == nil {
		t.Fatal("migrate with a shorter list succeeded, want downgrade error")
	}
}

func assertTaskCount(t *testing.T, store *Store, want int) {
	t.Helper()
	var count int
	if err := store.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM review_tasks`).Scan(&count); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if count != want {
		t.Errorf("task count = %d, want %d", count, want)
	}
}
