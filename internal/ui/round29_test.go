package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/state"
)

// A fleet of two tools says which is which, with the model each last
// answered with: "opencode · mock-1" beside "claude · opus-4-1" on the
// column's tag row and the card; a fleet of claudes with no model known
// says nothing extra (#50).
func TestTheTagSaysWhichToolAndWhichModel(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	if got := m.toolTag(m.sessions[m.selectedIndex()]); got != "" {
		t.Fatalf("a fleet of claudes with no model wears %q", got)
	}
	api := sessionKey("s-api")
	for i := range m.sessions {
		switch m.sessions[i].Info.Key() {
		case api:
			m.sessions[i].Info.Model = "claude-opus-4-1-20250805"
		case sessionKey("s-webapp"):
			m.sessions[i].Info.Tool, m.sessions[i].Info.Model = "opencode", "anthropic/claude-sonnet-4-5"
		}
	}
	col := strings.Join(m.boardColumn(api, rowFor(t, m, api), 60, 20), "\n")
	if !strings.Contains(col, "claude · opus-4-1 · ⌁ dev:1.0") {
		t.Errorf("the claude column does not say its tool and model:\n%s", col)
	}
	web := sessionKey("s-webapp")
	col = strings.Join(m.boardColumn(web, rowFor(t, m, web), 60, 20), "\n")
	if !strings.Contains(col, "opencode · sonnet-4-5 · ⌁ dev:2.1") {
		t.Errorf("the opencode column does not say its tool and model:\n%s", col)
	}
	m.point(web)
	openTrail(m)
	if card := strings.Join(m.sessionCard(70), "\n"); !strings.Contains(card, "opencode · sonnet-4-5") {
		t.Errorf("the card does not carry the tool and model:\n%s", card)
	}
	// Narrow, the digest wins the row over the tool: the tag alone stays.
	col = strings.Join(m.boardColumn(web, rowFor(t, m, web), 34, 20), "\n")
	if strings.Contains(col, "opencode") && !strings.Contains(col, "⌁ dev:2.1") {
		t.Errorf("a narrow column lost the pane for the tool:\n%s", col)
	}
}

// The model's short name: the vendor's prefix and the release date go.
func TestShortModelNames(t *testing.T) {
	for in, want := range map[string]string{
		"claude-opus-4-1-20250805": "opus-4-1",
		"claude-sonnet-4-5":        "sonnet-4-5",
		"mock/mock-1":              "mock-1",
		"anthropic/claude-opus-5":  "opus-5",
		"":                         "",
	} {
		if got := shortModel(in); got != want {
			t.Errorf("shortModel(%q) = %q, want %q", in, got, want)
		}
	}
}

// An opencode session is a session: the same states, the same row, its
// tool named on the narrow list where the pane tag would go.
func TestAnOpencodeSessionIsARow(t *testing.T) {
	forceASCII(t)
	m := boardModel(100, 30)
	now := fixtureBase.Add(40 * time.Minute)
	s := fleet.Session{Info: fleet.SessionInfo{ID: "ses_x", TranscriptPath: "opencode://ses_x", ProjectSlug: "opencode",
		CWD: "/home/user/ocproj", OriginCWD: "/home/user/ocproj", Title: "run the gates", StartedAt: now.Add(-10 * time.Minute), LastEventAt: now.Add(-time.Minute),
		Tool: "opencode", Model: "mock/mock-1"},
		Snap: state.Snapshot{State: state.Working, Since: now.Add(-time.Minute), Reason: "tool call in flight", Activity: "Bash: pytest -q"}, Live: true}
	m.sessions = append(m.sessions, s)
	m.SetSessions(m.sessions, now)
	m.point(s.Info.Key())
	list := ansi.Strip(strings.Join(m.fleetLines(40, 28), "\n"))
	if !strings.Contains(list, "ocproj") || !strings.Contains(list, "opencode · mock-1") {
		t.Errorf("the narrow list does not name the opencode session and its tool:\n%s", list)
	}
}

// The page key sheds after the attach aside (#51, #52): pinned, since the
// fold went missing once between two decisions with nothing holding it.
func TestThePageKeyOutlastsTheAttachAside(t *testing.T) {
	m := boardModel(152, 40)
	openTrail(m)
	m.level = levelWaypoints // the legs: the reader's own footer is the held case (#53)
	order := m.shedOrder(false)
	aside, page := -1, -1
	for i, o := range order {
		switch o {
		case attachHint:
			aside = i
		case " · ctrl+d/u half page":
			page = i
		}
	}
	if aside < 0 || page < 0 || aside > page {
		t.Fatalf("shed order ranks the aside %d and the page key %d: the aside must go first\n%q", aside, page, order)
	}
	m.inTmux = false
	m.View() // the footer is composed against the rows the frame drew (#199)
	if foot := ansi.Strip(m.footerLine(150)); !strings.Contains(foot, "ctrl+d/u half page") || strings.Contains(foot, "(prefix d returns)") {
		t.Errorf("the 152 legs footer should carry the page key and shed the aside: %q", foot)
	}
}
