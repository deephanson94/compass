package fleet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/deephanson94/compass/internal/transcript"
)

// workDirsMax is how many directories a session remembers working in: the
// most recent ones, which is where its pull requests are.
const workDirsMax = 32

// dirArg is a directory a shell command moves into or points git at:
// `cd ../repoB && git push`, `git -C /src/repoA push`.
var dirArg = regexp.MustCompile(`(?:^|[\s;&|(])(?:cd|pushd)\s+("[^"]+"|'[^']+'|[^\s;&|)]+)|\bgit\s+-C\s+("[^"]+"|'[^']+'|[^\s;&|)]+)`)

// workDirs is where a session's own tool calls worked, outside its cwd as
// much as in it: the directory of every file it edited or wrote, and every
// directory a shell command cd'd into or ran git in. A session started in a
// working dir above two repos pushes to both from there; its cwd names
// neither. A subagent's calls are its own.
func workDirs(ev transcript.Event, cwd string) []string {
	if ev.IsSidechain || len(ev.ToolUses) == 0 {
		return nil
	}
	if ev.CWD != "" {
		cwd = ev.CWD
	}
	var out []string
	add := func(p string) {
		p = strings.Trim(p, `"'`)
		if p == "" || p == "-" || strings.ContainsAny(p, "$`*?") {
			return // a variable or a glob names no one directory
		}
		if p == "~" || strings.HasPrefix(p, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return
			}
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
		if !filepath.IsAbs(p) {
			if cwd == "" {
				return
			}
			p = filepath.Join(cwd, p)
		}
		out = append(out, filepath.Clean(p))
	}
	for _, tu := range ev.ToolUses {
		switch tu.Name {
		case "Edit", "Write", "MultiEdit", "NotebookEdit":
			var in struct {
				FilePath     string `json:"file_path"`
				NotebookPath string `json:"notebook_path"`
			}
			if json.Unmarshal(tu.Input, &in) == nil {
				if p := in.FilePath + in.NotebookPath; p != "" && filepath.IsAbs(p) {
					add(filepath.Dir(p))
				}
			}
		case "Bash":
			var in struct {
				Command string `json:"command"`
			}
			if json.Unmarshal(tu.Input, &in) != nil {
				continue
			}
			for _, m := range dirArg.FindAllStringSubmatch(in.Command, -1) {
				add(m[1] + m[2])
			}
		}
	}
	return out
}

// noteWorkDirs keeps dirs as the most recent last, without repeats, at most
// workDirsMax long.
func noteWorkDirs(have []string, dirs ...string) []string {
	for _, d := range dirs {
		for i, h := range have {
			if h == d {
				have = append(have[:i], have[i+1:]...)
				break
			}
		}
		have = append(have, d)
	}
	if len(have) > workDirsMax {
		have = append([]string(nil), have[len(have)-workDirsMax:]...)
	}
	return have
}

// prRefsMax is how many pull requests a session remembers being pointed at.
const prRefsMax = 16

// prURL is a pull request named by its web address — a Gitea's
// https://host/owner/repo/pulls/4, a GitHub's /pull/4, under a sub-path or
// not — or by its API path, /repos/owner/repo/pulls/4.
var prURL = regexp.MustCompile(`(?:https?://[^\s"'<>()]+?|/repos)/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/pulls?/(\d+)`)

// prRefs is the pull requests a session was pointed at by name: an address
// in your prompt ("review https://gitea/org/repo/pulls/4"), or in a command
// or a fetch its tool calls made. A session reviewing a pull request often
// never checks its branch out, and this is the only thread from it to the
// pull request. Each is "owner/repo#n", lower case.
func prRefs(ev transcript.Event) []string {
	if ev.IsSidechain {
		return nil
	}
	var texts []string
	if ev.Type == transcript.EventUser && !ev.IsMeta && ev.Text != "" {
		texts = append(texts, ev.Text)
	}
	for _, tu := range ev.ToolUses {
		texts = append(texts, string(tu.Input))
	}
	var out []string
	for _, t := range texts {
		for _, m := range prURL.FindAllStringSubmatch(t, -1) {
			out = append(out, strings.ToLower(m[1]+"/"+m[2])+"#"+m[3])
		}
	}
	return out
}

// notePRRefs keeps refs as the most recent last, without repeats, at most
// prRefsMax long.
func notePRRefs(have []string, refs ...string) []string {
	have = noteWorkDirs(have, refs...)
	if len(have) > prRefsMax {
		have = append([]string(nil), have[len(have)-prRefsMax:]...)
	}
	return have
}
