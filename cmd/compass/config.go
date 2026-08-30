package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// config is what ~/.config/compass/config.toml may set. Every entry has a
// flag; a flag given on the command line always wins. The file is a flat
// `key = "value"` list — the TOML subset a hand and a text editor produce —
// and anything unreadable is simply ignored: compass runs fine with no
// configuration at all.
type config struct {
	Root     string // root = "~/.claude"
	Narrator string // narrator = "haiku" | "off" | any claude model
	Readonly bool   // readonly = true
}

// loadConfig reads the config file if there is one. $COMPASS_CONFIG overrides
// the location (and makes the file testable).
func loadConfig() config {
	path := os.Getenv("COMPASS_CONFIG")
	if path == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return config{}
		}
		path = filepath.Join(base, "compass", "config.toml")
	}

	f, err := os.Open(path)
	if err != nil {
		return config{}
	}
	defer f.Close()

	var c config
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, value, ok := configLine(sc.Text())
		if !ok {
			continue
		}
		switch key {
		case "root":
			c.Root = value
		case "narrator":
			c.Narrator = value
		case "readonly":
			c.Readonly = value == "true"
		}
	}
	return c
}

// configLine parses one `key = "value"` row; comments and blanks are nothing.
func configLine(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	key, value, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	if key == "" || value == "" {
		return "", "", false
	}
	return key, value, true
}
