package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// runConfig is `compass config`: where the file is, every key with what is
// in effect, and the lines compass could not use. `compass config init`
// writes a starter file with every key commented out at its default.
func runConfig(args []string, out, errOut io.Writer) int {
	switch {
	case len(args) == 0:
		printConfig(out, loadConfig())
		return 0
	case len(args) == 1 && args[0] == "init":
		path := configPath()
		if path == "" {
			fmt.Fprintln(errOut, "compass config init: no config directory (set $COMPASS_CONFIG)")
			return 1
		}
		if err := writeConfigTemplate(path); err != nil {
			fmt.Fprintln(errOut, "compass config init:", err)
			return 1
		}
		fmt.Fprintf(out, "wrote %s · every key is commented out; uncomment what you want\n", path)
		return 0
	}
	fmt.Fprintln(errOut, "usage: compass config [init]")
	return 2
}

// printConfig lists every key under its section: the value the file set,
// or the default compass falls back to, and what the key does.
func printConfig(w io.Writer, c config) {
	switch {
	case c.Path == "":
		fmt.Fprintln(w, "config: no config directory (set $COMPASS_CONFIG)")
	case c.Exists:
		fmt.Fprintln(w, "config:", homeShortPath(c.Path))
	default:
		fmt.Fprintf(w, "config: %s · not there yet (compass config init writes one)\n", homeShortPath(c.Path))
	}
	width := 0
	for _, k := range configKeys {
		width = max(width, len(k.name))
	}
	section := ""
	for _, k := range configKeys {
		if k.section != section {
			section = k.section
			fmt.Fprintf(w, "\n%s\n", section)
		}
		vals := k.get(c)
		if len(vals) == 0 {
			shown := "(unset)"
			if k.def != "" {
				shown = "(default) " + k.def
			}
			fmt.Fprintf(w, "  %-*s  %s\n", width, k.name, shown)
		}
		for _, v := range vals {
			fmt.Fprintf(w, "  %-*s  = %s\n", width, k.name, strconv.Quote(v))
		}
		fmt.Fprintf(w, "  %-*s    %s\n", width, "", k.help)
	}
	if len(c.Warnings) > 0 {
		fmt.Fprintln(w, "\nnot used:")
		for _, s := range c.Warnings {
			fmt.Fprintln(w, "  "+s)
		}
	}
}

// configTemplate is the starter file: every key in the table, under its
// section, its help as a comment and the line itself commented out at its
// default — or at an example where compass has none.
func configTemplate() string {
	var b strings.Builder
	b.WriteString("# compass config. Every key compass reads is below, commented out.\n")
	b.WriteString("# Uncomment a line and change its value; `compass config` shows what is in effect,\n")
	b.WriteString("# and lists any line compass could not use. A flag on the command line always wins.\n")
	section := ""
	for _, k := range configKeys {
		if k.section != section {
			section = k.section
			fmt.Fprintf(&b, "\n# ── %s\n", section)
		}
		v := k.def
		if v == "" {
			v = k.example
		}
		fmt.Fprintf(&b, "\n# %s\n", k.help)
		if k.repeat {
			b.WriteString("# (one line each; repeat the key for more)\n")
		}
		fmt.Fprintf(&b, "# %s = %s\n", k.name, strconv.Quote(v))
	}
	return b.String()
}

// writeConfigTemplate writes the starter file, and never over one: a config
// someone wrote is theirs.
func writeConfigTemplate(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s is already there · compass config shows what it sets", homeShortPath(path))
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(configTemplate()); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// homeShortPath writes the home directory as ~.
func homeShortPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}

// configWarning is the one-line version of the warnings, for the deck's
// footer and a command's stderr: the first, and how many more.
func configWarning(c config) string {
	if len(c.Warnings) == 0 {
		return ""
	}
	s := "config " + c.Warnings[0]
	if n := len(c.Warnings) - 1; n > 0 {
		s += fmt.Sprintf(" · %d more", n)
	}
	return s + " · compass config"
}
