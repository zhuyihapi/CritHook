package review

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestParseInvocation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    Invocation
		wantErr error
	}{
		{
			name: "bare command",
			body: "/review",
			want: Invocation{},
		},
		{
			name: "command with trailing whitespace",
			body: "/review   ",
			want: Invocation{},
		},
		{
			name: "focus text",
			body: "/review look at error handling",
			want: Invocation{Focus: "look at error handling"},
		},
		{
			name: "agent option with value",
			body: "/review --agent pi",
			want: Invocation{AgentName: "pi"},
		},
		{
			name: "agent option with equals value",
			body: "/review --agent=pi concurrency",
			want: Invocation{AgentName: "pi", Focus: "concurrency"},
		},
		{
			name: "agent option before multi-line focus",
			body: "/review --agent pi\nfocus line one\nfocus line two",
			want: Invocation{AgentName: "pi", Focus: "focus line one\nfocus line two"},
		},
		{
			name: "focus preserves interior whitespace",
			body: "/review focus   with  double  spaces",
			want: Invocation{Focus: "focus   with  double  spaces"},
		},
		{
			name: "focus preserves shell syntax as data",
			body: "/review check $(pwd) `id` ; rm -rf / && echo pwned | sh",
			want: Invocation{Focus: "check $(pwd) `id` ; rm -rf / && echo pwned | sh"},
		},
		{
			name: "options after focus are focus text",
			body: "/review focus --agent evil",
			want: Invocation{Focus: "focus --agent evil"},
		},
		{
			name:    "agent option missing value",
			body:    "/review --agent",
			wantErr: ErrMalformed,
		},
		{
			name:    "agent option empty value",
			body:    "/review --agent=",
			wantErr: ErrMalformed,
		},
		{
			name:    "repeated agent option",
			body:    "/review --agent pi --agent codex",
			wantErr: ErrMalformed,
		},
		{
			name:    "unknown option",
			body:    "/review --verbose focus",
			wantErr: ErrMalformed,
		},
		{
			name:    "exec-looking option",
			body:    "/review --exec rm -rf /",
			wantErr: ErrMalformed,
		},
		{
			name:    "empty body",
			body:    "",
			wantErr: ErrNotCommand,
		},
		{
			name:    "longer command token",
			body:    "/reviewx focus",
			wantErr: ErrNotCommand,
		},
		{
			name:    "command mid-comment",
			body:    "please /review this",
			wantErr: ErrNotCommand,
		},
		{
			name:    "leading whitespace",
			body:    " /review focus",
			wantErr: ErrNotCommand,
		},
		{
			name:    "uppercase command token",
			body:    "/Review focus",
			wantErr: ErrNotCommand,
		},
		{
			name:    "quoted command",
			body:    "> /review focus",
			wantErr: ErrNotCommand,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseInvocation(tt.body)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParseInvocation(%q) error = %v, want %v", tt.body, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseInvocation(%q) unexpected error: %v", tt.body, err)
			}
			if got != tt.want {
				t.Fatalf("ParseInvocation(%q) = %+v, want %+v", tt.body, got, tt.want)
			}
		})
	}
}

func TestResolveAgent(t *testing.T) {
	cfg := AgentConfig{Default: "codex", Available: []string{"pi", "codex"}}

	tests := []struct {
		name    string
		inv     Invocation
		cfg     AgentConfig
		want    string
		wantErr error
	}{
		{
			name: "explicit configured agent",
			inv:  Invocation{AgentName: "pi", Focus: "x"},
			cfg:  cfg,
			want: "pi",
		},
		{
			name: "omitted agent selects default",
			inv:  Invocation{Focus: "x"},
			cfg:  cfg,
			want: "codex",
		},
		{
			name:    "unknown agent",
			inv:     Invocation{AgentName: "claude"},
			cfg:     cfg,
			wantErr: ErrUnknownAgent,
		},
		{
			name:    "agent name is case sensitive",
			inv:     Invocation{AgentName: "PI"},
			cfg:     cfg,
			wantErr: ErrUnknownAgent,
		},
		{
			name:    "no default configured",
			inv:     Invocation{Focus: "x"},
			cfg:     AgentConfig{Available: []string{"pi"}},
			wantErr: ErrNoDefaultAgent,
		},
		{
			name:    "empty configuration",
			inv:     Invocation{AgentName: "pi"},
			cfg:     AgentConfig{},
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "default not available",
			inv:     Invocation{Focus: "x"},
			cfg:     AgentConfig{Default: "gone", Available: []string{"pi"}},
			wantErr: ErrInvalidConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.inv.ResolveAgent(tt.cfg)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ResolveAgent() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveAgent() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveAgent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAgentConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     AgentConfig
		wantErr bool
	}{
		{name: "valid", cfg: AgentConfig{Default: "pi", Available: []string{"pi", "codex"}}},
		{name: "valid without default", cfg: AgentConfig{Available: []string{"pi"}}},
		{name: "no agents", cfg: AgentConfig{}, wantErr: true},
		{name: "empty agent name", cfg: AgentConfig{Available: []string{"pi", ""}}, wantErr: true},
		{name: "duplicate agent", cfg: AgentConfig{Available: []string{"pi", "pi"}}, wantErr: true},
		{name: "default missing from available", cfg: AgentConfig{Default: "codex", Available: []string{"pi"}}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestFocusShellSyntaxIsDataNotExecuted guards the contract that focus text
// is data only: parsing and resolving a comment whose focus embeds shell
// syntax must leave the referenced files untouched.
func TestFocusShellSyntaxIsDataNotExecuted(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	body := "/review run $(rm -f " + marker + ") `rm -f " + marker + "` ; rm -f " + marker
	inv, err := ParseInvocation(body)
	if err != nil {
		t.Fatalf("ParseInvocation() unexpected error: %v", err)
	}
	if _, err := inv.ResolveAgent(AgentConfig{Default: "pi", Available: []string{"pi"}}); err != nil {
		t.Fatalf("ResolveAgent() unexpected error: %v", err)
	}

	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("focus text was executed: %v", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "keep" {
		t.Fatalf("marker modified: %q, %v", got, err)
	}
}
