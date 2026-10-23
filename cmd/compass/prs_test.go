package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/prstate"
)

type teaTable map[string]string

func (t teaTable) Output(name string, args ...string) ([]byte, error) {
	if out, ok := t[name+" "+strings.Join(args, " ")]; ok {
		return []byte(out), nil
	}
	return nil, errors.New("no answer")
}

// `compass prs` says, for each live session, where it looked, the checkout
// it found, and what it shows — and for each PR its verdict.
func TestPrintPRsWalksEveryStep(t *testing.T) {
	now := time.Date(2026, 10, 22, 11, 0, 0, 0, time.UTC)
	run := teaTable{
		"git -C /work rev-parse --show-toplevel":                                   "/work\n",
		"git -C /work rev-parse --abbrev-ref HEAD":                                 "dev/x\n",
		"git -C /work remote get-url origin":                                       "git@gitea:org/repo.git\n",
		"git -C /home rev-parse --show-toplevel":                                   "",
		"tea api /user":                                                            `{"login":"me"}`,
		"tea api /repos/issues/search?type=pulls&created=true&state=open&limit=50": `[{"number":5,"updated_at":"u","repository":{"full_name":"org/repo"}}]`,
		"tea api /repos/org/repo/pulls/5":                                          `{"number":5,"state":"open","user":{"login":"me"},"head":{"ref":"dev/x"},"requested_reviewers":[{"login":"bob"}]}`,
		"tea api /repos/org/repo/pulls/5/reviews":                                  `[]`,
		"tea api /repos/org/repo/issues/5/comments":                                `[{"user":{"login":"bob"},"created_at":"2026-10-22T10:00:00Z"}]`,
	}
	tr := prstate.New(run, "", time.Minute, "")
	sessions := []fleet.Session{
		{Live: true, Info: fleet.SessionInfo{ID: "aaaaaaaa1", TranscriptPath: "/t/a.jsonl", CWD: "/work", Name: "api"}},
		{Live: true, Info: fleet.SessionInfo{ID: "bbbbbbbb2", TranscriptPath: "/t/b.jsonl", CWD: "/nowhere/home"}},
	}
	var out bytes.Buffer
	printPRs(&out, sessions, tr, now)
	got := out.String()
	for _, want := range []string{
		"tea answers as me",
		"searches: 1 opened by you · 0 asking your review · 0 you reviewed",
		"api (aaaaaaaa)\n    looks in: /work\n    checkout: org/repo @ dev/x\n    shows: PR #5 · 1 new comment 1h ago",
		"home (bbbbbbbb)\n    looks in: /nowhere/home\n    checkouts: none",
		"org/repo#5 @ dev/x",
		"errors:", // the review searches have no answer here
		"status line: PR▲1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in:\n%s", want, got)
		}
	}
}
