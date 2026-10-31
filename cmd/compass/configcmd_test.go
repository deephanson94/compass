package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The starter file is the table written out: uncommenting every key line in
// it is a config compass reads whole — every key taken, nothing reported.
// A key added to the table without a default or an example that its own
// set accepts fails here.
func TestConfigTemplateRoundTrips(t *testing.T) {
	keyLine := regexp.MustCompile(`^# ([a-z_]+) = `)
	var body strings.Builder
	for _, line := range strings.Split(configTemplate(), "\n") {
		if keyLine.MatchString(line) {
			line = strings.TrimPrefix(line, "# ")
		}
		body.WriteString(line + "\n")
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPASS_CONFIG", path)
	c := loadConfig()
	if len(c.Warnings) > 0 {
		t.Errorf("the starter file, uncommented, is not read whole: %q", c.Warnings)
	}
	for _, k := range configKeys {
		if !c.has(k.name) {
			t.Errorf("%s is in the table but the starter file does not set it", k.name)
		}
		if len(k.get(c)) == 0 {
			t.Errorf("%s was read but get reports nothing: compass config would call it unset", k.name)
		}
		if k.help == "" || k.section == "" {
			t.Errorf("%s has no help or no section", k.name)
		}
	}
}

// Every key the table knows is in the README's config block, so the
// documentation cannot fall behind a key added later.
func TestReadmeNamesEveryConfigKey(t *testing.T) {
	raw, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range configKeys {
		if !regexp.MustCompile(`(?m)^` + k.name + ` = `).Match(raw) {
			t.Errorf("README's config block has no %q line", k.name+" = ")
		}
	}
}

// A key compass does not know is reported with the nearest one it does.
func TestUnknownKeysAreReportedWithAGuess(t *testing.T) {
	for typo, want := range map[string]string{
		"new_thn":    "new_then",
		"newthen":    "new_then",
		"card_with":  "card_width",
		"hung_after": "attn_after", // an old name is known, not a typo, but resolves to the new one
		"jmp":        "jump",
		"favourite":  "",
	} {
		if got := suggestKey(typo); got != want {
			t.Errorf("suggestKey(%q) = %q, want %q", typo, got, want)
		}
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	body := "new_thn = \"go\"\nnew_then = \"maybe\"\nreadonly = yes\nhung_after = \"2h\"\nnew_run = \"opencode\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPASS_CONFIG", path)
	c := loadConfig()
	want := []string{
		`line 1: unknown key "new_thn" (did you mean new_then?)`,
		`line 2: new_then = "maybe": want go or stay`,
		`line 3: readonly = "yes": want true or false`,
		`new_run = "opencode": no such harness (want claude, shell)`,
	}
	if strings.Join(c.Warnings, "\n") != strings.Join(want, "\n") {
		t.Errorf("warnings\n got %q\nwant %q", c.Warnings, want)
	}
	if c.AttnAfter != "2h" || c.NewThen != "" || c.NewRun != "" {
		t.Errorf("attn_after %q, new_then %q, new_run %q: the old name should count, the bad values should not", c.AttnAfter, c.NewThen, c.NewRun)
	}
	if got := configWarning(c); !strings.HasPrefix(got, `config line 1: unknown key "new_thn"`) || !strings.Contains(got, "3 more · compass config") {
		t.Errorf("footer line = %q", got)
	}
}

// compass config says where the file is and what is in effect; init writes
// the starter file once and never over a file that is there.
func TestCompassConfigCommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.toml")
	t.Setenv("COMPASS_CONFIG", path)
	var out, errOut bytes.Buffer
	if rc := runConfig(nil, &out, &errOut); rc != 0 || !strings.Contains(out.String(), "not there yet") || !regexp.MustCompile(`jump +\(default\) choose-tree -Zs`).MatchString(out.String()) {
		t.Errorf("rc %d, out:\n%s", rc, out.String())
	}
	out.Reset()
	if rc := runConfig([]string{"init"}, &out, &errOut); rc != 0 {
		t.Fatalf("init rc %d: %s", rc, errOut.String())
	}
	if raw, err := os.ReadFile(path); err != nil || string(raw) != configTemplate() {
		t.Fatalf("init wrote %q (%v)", raw, err)
	}
	if err := os.WriteFile(path, []byte("new_then = \"go\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	errOut.Reset()
	if rc := runConfig([]string{"init"}, &out, &errOut); rc == 0 || !strings.Contains(errOut.String(), "already there") {
		t.Errorf("init over a file: rc %d, %q", rc, errOut.String())
	}
	if raw, _ := os.ReadFile(path); string(raw) != "new_then = \"go\"\n" {
		t.Errorf("init overwrote the file: %q", raw)
	}
	out.Reset()
	runConfig(nil, &out, &errOut)
	if !regexp.MustCompile(`new_then +=  ?"go"`).MatchString(out.String()) {
		t.Errorf("compass config does not show the file's value:\n%s", out.String())
	}
}
