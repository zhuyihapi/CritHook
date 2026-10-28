// Package store persists accepted review tasks in SQLite.
//
// # Durable webhook acceptance boundary
//
// AcceptTask is the only entry point that creates tasks, and it commits the
// task and its delivery record in one transaction before returning. A webhook
// may be acknowledged as accepted only when AcceptTask returns a nil error;
// duplicates are already durable from their first delivery and are
// acknowledged the same way. On any error nothing was stored and the caller
// must fail the webhook so the platform can redeliver.
//
// # Delivery deduplication
//
// Platform deliveries are deduplicated on two stable identities, each scoped
// to its provider and project: the platform delivery identifier, when the
// platform provides one, and the identity of the triggering comment
// (review.TaskInput.SourceIdentity). Same names on another platform or
// another project never collide. A redelivery — same delivery ID or same
// triggering comment — returns the already accepted task without creating
// work. A different comment is a new command and is accepted again even for
// the same issue.
//
// # Task states
//
// Tasks are created in the queued state and move through an explicit
// lifecycle: queued (accepted, waiting for the executor), running (an
// executor is working on it), succeeded, and failed (both terminal). The
// schema constrains the stored values; execution transitions and restart
// recovery are owned by the execution lifecycle on top of this store.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	"github.com/zhuyihapi/CritHook/internal/review"

	_ "modernc.org/sqlite"
)

// TaskState is the lifecycle state of a persisted review task.
type TaskState string

const (
	// StateQueued marks an accepted task waiting for execution.
	StateQueued TaskState = "queued"
	// StateRunning marks a task currently held by an executor.
	StateRunning TaskState = "running"
	// StateSucceeded marks a task whose review completed.
	StateSucceeded TaskState = "succeeded"
	// StateFailed marks a task whose execution or timeout failed.
	StateFailed TaskState = "failed"
)

// AcceptOutcome reports what AcceptTask did with a delivery.
type AcceptOutcome string

const (
	// OutcomeAccepted means a new queued task was created.
	OutcomeAccepted AcceptOutcome = "accepted"
	// OutcomeDuplicateDelivery means the platform delivery identifier was
	// already recorded for this provider and project.
	OutcomeDuplicateDelivery AcceptOutcome = "duplicate-delivery"
	// OutcomeDuplicateCommand means the triggering comment was already
	// accepted, under a delivery identifier not seen before.
	OutcomeDuplicateCommand AcceptOutcome = "duplicate-command"
)

// Task is a persisted review task: its normalized input, lifecycle state, and
// storage timestamps in UTC.
type Task struct {
	ID        int64
	Input     review.TaskInput
	State     TaskState
	CreatedAt time.Time
	UpdatedAt time.Time
}

const timeFormat = time.RFC3339Nano

// Store is a SQLite-backed task store. It serializes access on a single
// connection, matching the single-instance serial execution design.
type Store struct {
	db *sql.DB
}

// Open opens the SQLite database at path, creating it when missing, and
// applies pending migrations before returning. The path must name a
// persistent database file: empty paths and ":memory:" are rejected, because
// SQLite would otherwise provide a private temporary or in-memory database
// whose contents disappear when the connection closes, breaking the durable
// acceptance boundary. Commits are fully durable (WAL journal with
// synchronous=FULL) so an acknowledged task survives a crash. The context
// bounds connection setup and migration.
func Open(ctx context.Context, path string) (*Store, error) {
	dsn, err := dataSourceName(path)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: ping %s: %w", path, err)
	}
	if err := migrate(ctx, db, migrations); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// dataSourceName builds the SQLite URI for a persistent database file at
// path. The filesystem path is resolved to an absolute path and encoded as a
// URI path, so reserved characters in the filename (for example '#', '?',
// and '%') cannot alter which file SQLite opens; the pragma query is fixed
// input assembled here, never derived from the path.
func dataSourceName(path string) (string, error) {
	if path == "" || path == ":memory:" {
		return "", fmt.Errorf("store: non-persistent database path %q", path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("store: resolve database path %q: %w", path, err)
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	return u.String() + "?" +
		"_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(FULL)" +
		"&_pragma=foreign_keys(1)", nil
}

// Close releases the database connection.
func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("store: close: %w", err)
	}
	return nil
}

// AcceptTask durably records one normalized review request before its webhook
// is acknowledged. It returns the stored task and what happened to the
// delivery: a new command is created as a queued task, a redelivery returns
// the already accepted task, and an invalid input is rejected without
// storing anything. On error nothing was stored and the webhook must not be
// acknowledged as accepted.
func (s *Store) AcceptTask(ctx context.Context, in review.TaskInput) (Task, AcceptOutcome, error) {
	if err := in.Validate(); err != nil {
		return Task{}, "", err
	}

	var task Task
	var outcome AcceptOutcome
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if in.DeliveryID != "" {
			found, err := taskByDelivery(ctx, tx, in)
			if err != nil {
				return err
			}
			if found.ID != 0 {
				task, outcome = found, OutcomeDuplicateDelivery
				return nil
			}
		}

		found, err := taskBySource(ctx, tx, in.SourceIdentity())
		if err != nil {
			return err
		}
		if found.ID != 0 {
			if in.DeliveryID != "" {
				if err := recordDelivery(ctx, tx, in, in.SourceIdentity()); err != nil {
					return err
				}
			}
			task, outcome = found, OutcomeDuplicateCommand
			return nil
		}

		created, err := insertTask(ctx, tx, in)
		if err != nil {
			return err
		}
		if in.DeliveryID != "" {
			if err := recordDelivery(ctx, tx, in, in.SourceIdentity()); err != nil {
				return err
			}
		}
		task, outcome = created, OutcomeAccepted
		return nil
	})
	if err != nil {
		return Task{}, "", err
	}
	return task, outcome, nil
}

// TaskBySource returns the task accepted for a source command identity. It
// returns an error wrapping sql.ErrNoRows when no task matches.
func (s *Store) TaskBySource(ctx context.Context, identity string) (Task, error) {
	row := s.db.QueryRowContext(ctx, taskSelectSQL+`WHERE source_identity = ?`, identity)
	task, err := scanTask(row)
	if err != nil {
		return Task{}, fmt.Errorf("store: task by source %q: %w", identity, err)
	}
	return task, nil
}

// TasksByState returns all tasks in state, oldest first. The state must be a
// known TaskState.
func (s *Store) TasksByState(ctx context.Context, state TaskState) ([]Task, error) {
	switch state {
	case StateQueued, StateRunning, StateSucceeded, StateFailed:
	default:
		return nil, fmt.Errorf("store: unknown task state %q", state)
	}

	rows, err := s.db.QueryContext(ctx, taskSelectSQL+`WHERE state = ? ORDER BY id`, state)
	if err != nil {
		return nil, fmt.Errorf("store: tasks by state %q: %w", state, err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("store: tasks by state %q: %w", state, err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: tasks by state %q: %w", state, err)
	}
	return tasks, nil
}

func (s *Store) withTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin: %w", err)
	}
	if err := fn(tx); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("store: %w (rollback: %w)", err, rollbackErr)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}
	return nil
}

const taskSelectSQL = `SELECT id, source_identity, provider, owner, repo,
	external_project_id, review_kind, number, comment_id, requester_login,
	agent, focus, delivery_id, state, received_at, created_at, updated_at
	FROM review_tasks `

func taskByDelivery(ctx context.Context, tx *sql.Tx, in review.TaskInput) (Task, error) {
	var sourceIdentity string
	err := tx.QueryRowContext(ctx,
		`SELECT source_identity FROM task_deliveries
		WHERE provider = ? AND owner = ? AND repo = ? AND delivery_id = ?`,
		string(in.Project.Provider), in.Project.Owner, in.Project.Repo, in.DeliveryID,
	).Scan(&sourceIdentity)
	if err == sql.ErrNoRows {
		return Task{}, nil
	}
	if err != nil {
		return Task{}, fmt.Errorf("store: delivery lookup: %w", err)
	}
	row := tx.QueryRowContext(ctx, taskSelectSQL+`WHERE source_identity = ?`, sourceIdentity)
	task, err := scanTask(row)
	if err != nil {
		return Task{}, fmt.Errorf("store: task for recorded delivery %q: %w", in.DeliveryID, err)
	}
	return task, nil
}

func taskBySource(ctx context.Context, tx *sql.Tx, identity string) (Task, error) {
	row := tx.QueryRowContext(ctx, taskSelectSQL+`WHERE source_identity = ?`, identity)
	task, err := scanTask(row)
	if err == sql.ErrNoRows {
		return Task{}, nil
	}
	if err != nil {
		return Task{}, fmt.Errorf("store: task lookup by source %q: %w", identity, err)
	}
	return task, nil
}

func insertTask(ctx context.Context, tx *sql.Tx, in review.TaskInput) (Task, error) {
	now := nowUTC()
	received := ""
	if !in.ReceivedAt.IsZero() {
		received = in.ReceivedAt.UTC().Format(timeFormat)
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO review_tasks (
		source_identity, provider, owner, repo, external_project_id,
		review_kind, number, comment_id, requester_login, agent, focus,
		delivery_id, state, received_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.SourceIdentity(),
		string(in.Project.Provider), in.Project.Owner, in.Project.Repo, in.Project.ExternalID,
		string(in.ReviewKind), in.Number, in.CommentID, in.Requester.Login,
		in.Command.Agent, in.Command.Focus,
		in.DeliveryID, string(StateQueued), received,
		now.Format(timeFormat), now.Format(timeFormat),
	)
	if err != nil {
		return Task{}, fmt.Errorf("store: insert task %q: %w", in.SourceIdentity(), err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Task{}, fmt.Errorf("store: insert task %q: last insert id: %w", in.SourceIdentity(), err)
	}
	row := tx.QueryRowContext(ctx, taskSelectSQL+`WHERE id = ?`, id)
	task, err := scanTask(row)
	if err != nil {
		return Task{}, fmt.Errorf("store: load inserted task %q: %w", in.SourceIdentity(), err)
	}
	return task, nil
}

func recordDelivery(ctx context.Context, tx *sql.Tx, in review.TaskInput, sourceIdentity string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO task_deliveries (
		provider, owner, repo, delivery_id, source_identity, first_seen_at
	) VALUES (?, ?, ?, ?, ?, ?)`,
		string(in.Project.Provider), in.Project.Owner, in.Project.Repo,
		in.DeliveryID, sourceIdentity, nowUTC().Format(timeFormat),
	)
	if err != nil {
		return fmt.Errorf("store: record delivery %q: %w", in.DeliveryID, err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTask(row rowScanner) (Task, error) {
	var t Task
	var discard string
	var provider, kind, state string
	var received, created, updated string
	if err := row.Scan(
		&t.ID, &discard,
		&provider, &t.Input.Project.Owner, &t.Input.Project.Repo, &t.Input.Project.ExternalID,
		&kind, &t.Input.Number, &t.Input.CommentID, &t.Input.Requester.Login,
		&t.Input.Command.Agent, &t.Input.Command.Focus,
		&t.Input.DeliveryID, &state, &received, &created, &updated,
	); err != nil {
		return Task{}, err
	}
	t.Input.Project.Provider = review.Provider(provider)
	t.Input.ReviewKind = review.ReviewKind(kind)
	t.State = TaskState(state)

	var err error
	if t.CreatedAt, err = time.Parse(timeFormat, created); err != nil {
		return Task{}, fmt.Errorf("store: task %d created_at: %w", t.ID, err)
	}
	if t.UpdatedAt, err = time.Parse(timeFormat, updated); err != nil {
		return Task{}, fmt.Errorf("store: task %d updated_at: %w", t.ID, err)
	}
	if received != "" {
		receivedAt, err := time.Parse(timeFormat, received)
		if err != nil {
			return Task{}, fmt.Errorf("store: task %d received_at: %w", t.ID, err)
		}
		t.Input.ReceivedAt = receivedAt
	}
	return t, nil
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
