package main

import "testing"

// -e takes KEY=VALUE, the key a variable's name and the value anything,
// "=" included; anything else is refused before tmux is asked.
func TestNewEnvFlag(t *testing.T) {
	var e envFlag
	for _, ok := range []string{"ATTENTION_NO_IDLE=1", "CLAUDE_CODE_USE_BEDROCK=1", "OPTS=a=b c", "EMPTY="} {
		if err := e.Set(ok); err != nil {
			t.Errorf("Set(%q) = %v, want it taken", ok, err)
		}
	}
	if len(e) != 4 || e[2] != "OPTS=a=b c" {
		t.Errorf("env = %q", e)
	}
	for _, bad := range []string{"NOEQUALS", "=value", "1ABC=x", "MY-VAR=x", "A B=x"} {
		if err := e.Set(bad); err == nil {
			t.Errorf("Set(%q) was taken", bad)
		}
	}
}
