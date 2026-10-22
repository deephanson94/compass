package prstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// Runner executes `<name> <args...>` and returns stdout. Every call compass
// makes to `git` and `tea` goes through one, so the tests never touch a
// checkout or a network — the shape tmuxop.Runner gives tmux.
type Runner interface {
	Output(name string, args ...string) ([]byte, error)
}

// RealRunner shells out to the binary on PATH. A call that has not answered
// in Timeout is killed: a hung Gitea costs one poll, never the next.
type RealRunner struct {
	Timeout time.Duration
}

// Output runs the command. A missing binary surfaces as exec.ErrNotFound.
func (r RealRunner) Output(name string, args ...string) ([]byte, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}

// link is where a session's checkout points: the repository its origin
// names and the branch it has checked out.
type link struct {
	Host   string `json:"host"`
	Repo   string `json:"repo"` // owner/repo, lower case
	Branch string `json:"branch"`
}

// linkOf reads a checkout's branch and origin. A directory that is not a
// checkout, a detached HEAD, or an origin that names no owner/repo is no
// link: the session has no pull request to show.
func linkOf(r Runner, cwd string) (link, bool) {
	if cwd == "" {
		return link{}, false
	}
	out, err := r.Output("git", "-C", cwd, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return link{}, false
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" || branch == "HEAD" {
		return link{}, false
	}
	out, err = r.Output("git", "-C", cwd, "remote", "get-url", "origin")
	if err != nil {
		return link{}, false
	}
	host, repo, ok := ParseRemote(strings.TrimSpace(string(out)))
	if !ok {
		return link{}, false
	}
	return link{Host: host, Repo: repo, Branch: branch}, true
}

// ParseRemote splits a git remote into its host and owner/repo:
// git@host:owner/repo.git, ssh://git@host:2222/owner/repo.git and
// https://host/owner/repo all name the same pair. The repository is lower
// case, as Gitea compares it.
func ParseRemote(remote string) (host, repo string, ok bool) {
	var path string
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil {
			return "", "", false
		}
		host, path = u.Hostname(), u.Path
	} else if at, rest, found := strings.Cut(remote, ":"); found && !strings.Contains(at, "/") {
		// scp form: [user@]host:owner/repo.git
		if i := strings.LastIndexByte(at, '@'); i >= 0 {
			at = at[i+1:]
		}
		host, path = at, rest
	} else {
		return "", "", false
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	parts := strings.Split(path, "/")
	if host == "" || len(parts) < 2 || parts[len(parts)-2] == "" || parts[len(parts)-1] == "" {
		return "", "", false
	}
	// A Gitea served under a sub-path keeps owner/repo as the last two.
	repo = strings.ToLower(parts[len(parts)-2] + "/" + parts[len(parts)-1])
	return host, repo, true
}

// errNoTea says tea is not on this machine.
var errNoTea = errors.New("tea is not on PATH")

// api runs `tea api [--login L] <path>` and decodes its JSON into v. An
// answer that is not the shape asked for — Gitea's {"message": …} on a
// 404, a login prompt — is an error, never an empty result.
func api(r Runner, login, path string, v any) error {
	args := []string{"api"}
	if login != "" {
		args = append(args, "--login", login)
	}
	args = append(args, path)
	out, err := r.Output("tea", args...)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return errNoTea
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return fmt.Errorf("tea api %s: %s", path, strings.TrimSpace(string(ee.Stderr)))
		}
		return fmt.Errorf("tea api %s: %w", path, err)
	}
	if err := json.Unmarshal(out, v); err != nil {
		return fmt.Errorf("tea api %s: %s", path, firstLine(out))
	}
	return nil
}

func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 120 {
		s = s[:120] + "…"
	}
	return s
}
