// Package review defines the shared /review command syntax and the
// normalized, provider-independent task input that GitHub and GitLab
// ingestion produce from comment events.
//
// Comment content is untrusted data. Parsing only produces values; it never
// executes comment text, and an explicit agent selection is accepted only
// when it matches an executor configured on the server.
package review

import (
	"errors"
	"fmt"
	"strings"
)

// Errors reported when parsing or resolving a comment command. Callers use
// them to distinguish comments that are not commands (ignored silently)
// from recognizable commands that must be rejected without creating a task.
var (
	// ErrNotCommand reports a comment that does not start with the /review
	// command token.
	ErrNotCommand = errors.New("not a /review command")
	// ErrMalformed reports a recognizable /review command with invalid
	// options.
	ErrMalformed = errors.New("malformed /review command")
	// ErrUnknownAgent reports an explicit --agent name that is not
	// configured on the server.
	ErrUnknownAgent = errors.New("agent is not configured")
	// ErrNoDefaultAgent reports a command without --agent when the server
	// has no default executor configured.
	ErrNoDefaultAgent = errors.New("no default agent configured")
	// ErrInvalidConfig reports an AgentConfig that cannot select any
	// executor.
	ErrInvalidConfig = errors.New("invalid agent configuration")
)

// Command is the resolved review request carried by a task input. Agent is
// always a configured executor name and Focus is verbatim comment text.
type Command struct {
	Agent string
	Focus string
}

// Invocation is a syntactically parsed /review command. AgentName is the
// requested executor name, empty when the command omitted --agent. Focus is
// the remaining review focus text, preserved verbatim except for trailing
// whitespace.
type Invocation struct {
	AgentName string
	Focus     string
}

// AgentConfig names the executors a deployment offers. Default is selected
// when a command omits --agent and, when set, must be listed in Available.
type AgentConfig struct {
	Default   string
	Available []string
}

// Validate reports whether the configuration can select an executor: at
// least one non-empty unique agent name, and a Default that is available
// when set.
func (c AgentConfig) Validate() error {
	if len(c.Available) == 0 {
		return fmt.Errorf("%w: no available agents", ErrInvalidConfig)
	}
	seen := make(map[string]bool, len(c.Available))
	for _, name := range c.Available {
		if name == "" {
			return fmt.Errorf("%w: empty agent name", ErrInvalidConfig)
		}
		if seen[name] {
			return fmt.Errorf("%w: duplicate agent %q", ErrInvalidConfig, name)
		}
		seen[name] = true
	}
	if c.Default != "" && !seen[c.Default] {
		return fmt.Errorf("%w: default agent %q is not available", ErrInvalidConfig, c.Default)
	}
	return nil
}

// ResolveAgent resolves the invocation against the configured executors.
// An explicit AgentName must match an available name exactly; an empty
// AgentName selects Default. The returned name is always a configured
// executor, so comment content can never introduce another one.
func (inv Invocation) ResolveAgent(cfg AgentConfig) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", err
	}
	if inv.AgentName == "" {
		if cfg.Default == "" {
			return "", ErrNoDefaultAgent
		}
		return cfg.Default, nil
	}
	for _, name := range cfg.Available {
		if name == inv.AgentName {
			return name, nil
		}
	}
	return "", fmt.Errorf("%w: %q", ErrUnknownAgent, inv.AgentName)
}

// ParseInvocation parses a comment body as a /review command.
//
// The body is a command only when it starts with the literal token
// "/review" followed by whitespace or nothing. Options must precede the
// focus text; once a non-option token appears, everything to the end of the
// comment is focus text and is never interpreted as options. Only --agent
// and --agent=<name> are recognized; any other option is malformed.
func ParseInvocation(body string) (Invocation, error) {
	const command = "/review"
	if !strings.HasPrefix(body, command) {
		return Invocation{}, ErrNotCommand
	}
	rest := body[len(command):]
	if rest != "" && !isSpace(rest[0]) {
		return Invocation{}, ErrNotCommand
	}

	var inv Invocation
	agentSet := false
	pos := 0
	for pos < len(rest) {
		for pos < len(rest) && isSpace(rest[pos]) {
			pos++
		}
		if pos >= len(rest) {
			break
		}
		start := pos
		for pos < len(rest) && !isSpace(rest[pos]) {
			pos++
		}
		token := rest[start:pos]

		if !strings.HasPrefix(token, "--") {
			inv.Focus = strings.TrimRight(rest[start:], " \t\r\n")
			return inv, nil
		}

		value, ok, err := agentValue(rest, token, &pos)
		if err != nil {
			return Invocation{}, err
		}
		if !ok {
			return Invocation{}, fmt.Errorf("%w: unknown option %q", ErrMalformed, token)
		}
		if agentSet {
			return Invocation{}, fmt.Errorf("%w: repeated --agent", ErrMalformed)
		}
		inv.AgentName = value
		agentSet = true
	}
	return inv, nil
}

// agentValue consumes the value of an --agent option. It reports ok=false
// for tokens that are not an --agent option.
func agentValue(rest, token string, pos *int) (value string, ok bool, err error) {
	switch {
	case token == "--agent":
		p := *pos
		for p < len(rest) && isSpace(rest[p]) {
			p++
		}
		if p >= len(rest) {
			return "", true, fmt.Errorf("%w: --agent requires a value", ErrMalformed)
		}
		end := p
		for end < len(rest) && !isSpace(rest[end]) {
			end++
		}
		*pos = end
		return rest[p:end], true, nil
	case strings.HasPrefix(token, "--agent="):
		value := token[len("--agent="):]
		if value == "" {
			return "", true, fmt.Errorf("%w: --agent requires a value", ErrMalformed)
		}
		return value, true, nil
	default:
		return "", false, nil
	}
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
