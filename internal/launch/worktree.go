package launch

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Git runs `git -C dir <args...>` and returns stdout.
type Git interface {
	Output(dir string, args ...string) ([]byte, error)
}

// RealGit shells out to the git binary on PATH. A failure carries git's own
// complaint, which is what a person needs to read ("a branch named … already
// exists").
type RealGit struct{}

func (RealGit) Output(dir string, args ...string) ([]byte, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
		// The complaint is git's last line; a progress line ("Preparing
		// worktree…") can come before it.
		lines := strings.Split(strings.TrimSpace(string(ee.Stderr)), "\n")
		return out, fmt.Errorf("%s", strings.TrimPrefix(lines[len(lines)-1], "fatal: "))
	}
	return out, err
}

// Worktree checks out a new branch for dir's repository in a worktree of its
// own and returns the directory in it that corresponds to dir, so a launch
// from a package three levels down lands three levels down.
//
// Worktrees live beside the main checkout, never inside it — a directory in
// the tree is one more thing to ignore — in `<repo>.worktrees/<branch>`.
// The main checkout is found through the common git dir, so a launch from
// inside another worktree still puts the new one with its siblings rather
// than nesting it.
func Worktree(g Git, dir, branch string) (string, error) {
	top, err := g.Output(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("worktree: %s is not in a git repository", dir)
	}
	common, err := g.Output(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("worktree: %w", err)
	}
	root := strings.TrimSpace(string(top))
	main := filepath.Clean(strings.TrimSpace(string(common)))
	if filepath.Base(main) == ".git" {
		main = filepath.Dir(main) // a bare repository is its own common dir
	}
	path := filepath.Join(filepath.Dir(main), filepath.Base(main)+".worktrees", branch)
	if _, err := g.Output(root, "worktree", "add", "-b", branch, path); err != nil {
		return "", fmt.Errorf("worktree: %w", err)
	}
	// git names the top with its symlinks resolved; dir may still have its own.
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path, nil
	}
	return filepath.Join(path, rel), nil
}
