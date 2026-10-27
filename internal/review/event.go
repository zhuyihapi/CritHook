package review

import (
	"fmt"
	"strconv"
	"time"
)

// Provider identifies the platform an event came from.
type Provider string

const (
	ProviderGitHub Provider = "github"
	ProviderGitLab Provider = "gitlab"
)

// ReviewKind distinguishes an issue requirement review from a code review.
type ReviewKind string

const (
	// ReviewIssue reviews a GitHub or GitLab issue as a requirements
	// review.
	ReviewIssue ReviewKind = "issue"
	// ReviewCode reviews a GitHub pull request or GitLab merge request as
	// a code review.
	ReviewCode ReviewKind = "code"
)

// Project identifies the repository that hosts a review, scoped to its
// provider. Owner is the GitHub owner or the GitLab group/subgroup path.
// ExternalID is the platform's project ID when the payload provides one;
// older payloads may omit it.
type Project struct {
	Provider   Provider
	Owner      string
	Repo       string
	ExternalID string
}

// Requester is the account whose comment requested the review.
type Requester struct {
	Login string
}

// TaskInput is the normalized, provider-independent input for one accepted
// review request. Provider ingestion constructs it after signature,
// permission, and command validation; later stages never re-derive it.
type TaskInput struct {
	Project    Project
	ReviewKind ReviewKind
	// Number is the issue, pull request, or merge request number.
	Number    int
	CommentID string
	Requester Requester
	Command   Command
	// DeliveryID is the platform's delivery identifier, for example the
	// GitHub X-GitHub-Delivery header value. It is empty when the platform
	// provides none; deduplication relies on SourceIdentity instead.
	DeliveryID string
	// ReceivedAt is when ingestion accepted the request. The zero value
	// means unset.
	ReceivedAt time.Time
}

// SourceIdentity returns the stable identity of the triggering comment,
// scoped to its provider and project. Redeliveries of the same comment
// produce the same identity, which task persistence uses to deduplicate
// platform deliveries.
func (in TaskInput) SourceIdentity() string {
	return fmt.Sprintf("%s/%s/%s/%s/%s/comment/%s",
		in.Project.Provider,
		in.Project.Owner,
		in.Project.Repo,
		in.ReviewKind,
		strconv.Itoa(in.Number),
		in.CommentID,
	)
}

// Validate reports whether the input carries the identity- and
// execution-bearing fields later stages rely on: a known provider, a
// complete project, a supported review kind with a positive number, a
// comment identity, the requester login, and a resolved command.
// ExternalID, DeliveryID, and ReceivedAt are optional metadata.
func (in TaskInput) Validate() error {
	switch in.Project.Provider {
	case ProviderGitHub, ProviderGitLab:
	default:
		return fmt.Errorf("task input: unknown provider %q", in.Project.Provider)
	}
	if in.Project.Owner == "" || in.Project.Repo == "" {
		return fmt.Errorf("task input: incomplete project %s/%s", in.Project.Owner, in.Project.Repo)
	}
	switch in.ReviewKind {
	case ReviewIssue, ReviewCode:
	default:
		return fmt.Errorf("task input: unsupported review kind %q", in.ReviewKind)
	}
	if in.Number <= 0 {
		return fmt.Errorf("task input: non-positive %s number %d", in.ReviewKind, in.Number)
	}
	if in.CommentID == "" {
		return fmt.Errorf("task input: empty comment ID")
	}
	if in.Requester.Login == "" {
		return fmt.Errorf("task input: empty requester login")
	}
	if in.Command.Agent == "" {
		return fmt.Errorf("task input: unresolved command agent")
	}
	return nil
}
