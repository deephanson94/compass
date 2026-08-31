package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `# compass configuration
root = "~/claude-home"
narrator = 'off'
readonly = true
live_within = "90s"
mystery = "ignored"
this line is not a setting
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPASS_CONFIG", path)

	c := loadConfig()
	if c.Root != "~/claude-home" {
		t.Errorf("Root = %q", c.Root)
	}
	if c.Narrator != "off" {
		t.Errorf("Narrator = %q", c.Narrator)
	}
	if !c.Readonly {
		t.Error("Readonly = false, want true")
	}
	if c.LiveWithin != "90s" {
		t.Errorf("LiveWithin = %q", c.LiveWithin)
	}
}

func TestLoadConfigMissingFileIsNothing(t *testing.T) {
	t.Setenv("COMPASS_CONFIG", filepath.Join(t.TempDir(), "absent.toml"))
	if c := loadConfig(); c != (config{}) {
		t.Errorf("loadConfig() on a missing file = %+v, want zero", c)
	}
}

func TestConfigLine(t *testing.T) {
	cases := []struct {
		in       string
		key, val string
		ok       bool
	}{
		{`narrator = "haiku"`, "narrator", "haiku", true},
		{`readonly=true`, "readonly", "true", true},
		{`  root = '~/x'  `, "root", "~/x", true},
		{`# a comment`, "", "", false},
		{``, "", "", false},
		{`bare words`, "", "", false},
		{`= "orphan"`, "", "", false},
		{`empty = ""`, "", "", false},
	}
	for _, tc := range cases {
		key, val, ok := configLine(tc.in)
		if key != tc.key || val != tc.val || ok != tc.ok {
			t.Errorf("configLine(%q) = %q,%q,%v want %q,%q,%v", tc.in, key, val, ok, tc.key, tc.val, tc.ok)
		}
	}
}
