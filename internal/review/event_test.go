package review

import (
	"testing"
	"time"
)

func validGitHubIssueInput() TaskInput {
	return TaskInput{
		Project:    Project{Provider: ProviderGitHub, Owner: "zhuyihapi", Repo: "CritHook", ExternalID: "402012"},
		ReviewKind: ReviewIssue,
		Number:     42,
		CommentID:  "9912001",
		Requester:  Requester{Login: "alice"},
		Command:    Command{Agent: "pi", Focus: "security focus"},
		DeliveryID: "4c1a2a70-9d57-4bb1-a15f-89f6a68d3f2e",
		ReceivedAt: time.Date(2026, 10, 27, 7, 30, 0, 0, time.UTC),
	}
}

// TestTaskInputSupportsAllPlatformEventShapes constructs the normalized
// input for a GitHub issue comment, GitHub PR comment, GitLab issue
// comment, and GitLab MR comment. Each validates and produces its
// documented source identity.
func TestTaskInputSupportsAllPlatformEventShapes(t *testing.T) {
	tests := []struct {
		name string
		in   TaskInput
		want string
	}{
		{
			name: "GitHub issue comment",
			in:   validGitHubIssueInput(),
			want: "github/zhuyihapi/CritHook/issue/42/comment/9912001",
		},
		{
			name: "GitHub pull request comment",
			in: TaskInput{
				Project:    Project{Provider: ProviderGitHub, Owner: "zhuyihapi", Repo: "CritHook"},
				ReviewKind: ReviewCode,
				Number:     43,
				CommentID:  "9913002",
				Requester:  Requester{Login: "bob"},
				Command:    Command{Agent: "codex"},
			},
			want: "github/zhuyihapi/CritHook/code/43/comment/9913002",
		},
		{
			name: "GitLab issue comment",
			in: TaskInput{
				Project:    Project{Provider: ProviderGitLab, Owner: "group/subgroup", Repo: "crithook"},
				ReviewKind: ReviewIssue,
				Number:     7,
				CommentID:  "321",
				Requester:  Requester{Login: "carol"},
				Command:    Command{Agent: "pi"},
			},
			want: "gitlab/group/subgroup/crithook/issue/7/comment/321",
		},
		{
			name: "GitLab merge request comment",
			in: TaskInput{
				Project:    Project{Provider: ProviderGitLab, Owner: "group/subgroup", Repo: "crithook"},
				ReviewKind: ReviewCode,
				Number:     8,
				CommentID:  "322",
				Requester:  Requester{Login: "dave"},
				Command:    Command{Agent: "codex"},
			},
			want: "gitlab/group/subgroup/crithook/code/8/comment/322",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); err != nil {
				t.Fatalf("Validate() unexpected error: %v", err)
			}
			if got := tt.in.SourceIdentity(); got != tt.want {
				t.Fatalf("SourceIdentity() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSourceIdentityStability(t *testing.T) {
	base := validGitHubIssueInput()

	t.Run("same comment redelivered", func(t *testing.T) {
		redelivery := base
		redelivery.DeliveryID = "0e6ee55c-f9cb-4a11-9ee2-2e8d33b90f01"
		if base.SourceIdentity() != redelivery.SourceIdentity() {
			t.Fatalf(
				"identity changed across redelivery: %q vs %q",
				base.SourceIdentity(),
				redelivery.SourceIdentity(),
			)
		}
	})

	t.Run("different comment differs", func(t *testing.T) {
		other := base
		other.CommentID = "9912002"
		if base.SourceIdentity() == other.SourceIdentity() {
			t.Fatalf("identity collision between distinct comments: %q", base.SourceIdentity())
		}
	})

	t.Run("scoped to provider", func(t *testing.T) {
		mirror := base
		mirror.Project = Project{Provider: ProviderGitLab, Owner: "zhuyihapi", Repo: "CritHook"}
		if base.SourceIdentity() == mirror.SourceIdentity() {
			t.Fatalf("identity collision across providers: %q", base.SourceIdentity())
		}
	})
}

func TestTaskInputValidate(t *testing.T) {
	tests := []struct {
		name string
		mut  func(*TaskInput)
	}{
		{name: "unknown provider", mut: func(in *TaskInput) { in.Project.Provider = "gitea" }},
		{name: "missing owner", mut: func(in *TaskInput) { in.Project.Owner = "" }},
		{name: "missing repo", mut: func(in *TaskInput) { in.Project.Repo = "" }},
		{name: "unsupported review kind", mut: func(in *TaskInput) { in.ReviewKind = "snippet" }},
		{name: "zero number", mut: func(in *TaskInput) { in.Number = 0 }},
		{name: "negative number", mut: func(in *TaskInput) { in.Number = -1 }},
		{name: "empty comment ID", mut: func(in *TaskInput) { in.CommentID = "" }},
		{name: "empty requester login", mut: func(in *TaskInput) { in.Requester.Login = "" }},
		{name: "unresolved command agent", mut: func(in *TaskInput) { in.Command.Agent = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validGitHubIssueInput()
			tt.mut(&in)
			if err := in.Validate(); err == nil {
				t.Fatalf("Validate() = nil, want error")
			}
		})
	}

	t.Run("optional metadata may be empty", func(t *testing.T) {
		in := validGitHubIssueInput()
		in.Project.ExternalID = ""
		in.DeliveryID = ""
		in.ReceivedAt = time.Time{}
		if err := in.Validate(); err != nil {
			t.Fatalf("Validate() unexpected error: %v", err)
		}
	})
}
