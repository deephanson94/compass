package transcript

import "testing"

// A failed API call reaches the fleet whichever way the CLI wrote it: the
// flag inside the message, the flag on the line, the synthetic model, or
// nothing but the text a person sees on their screen. A session dead on
// quota sat in the fleet as a finished turn because only the first shape
// was read.
func TestAnAPIErrorIsReadInEveryShape(t *testing.T) {
	text := `"content":[{"type":"text","text":"Please run /login · API Error: 403 your daily quota is exhausted, contact your administrator."}]`
	for name, line := range map[string]string{
		"flag on the message": `{"type":"assistant","uuid":"1","timestamp":"2026-09-01T07:08:51Z","message":{"role":"assistant",` + text + `,"isApiErrorMessage":true,"apiErrorStatus":403,"error":"authentication_failed"}}`,
		"flag on the line":    `{"type":"assistant","uuid":"2","timestamp":"2026-09-01T07:08:51Z","isApiErrorMessage":true,"apiErrorStatus":403,"error":"authentication_failed","message":{"role":"assistant",` + text + `}}`,
		"synthetic model":     `{"type":"assistant","uuid":"3","timestamp":"2026-09-01T07:08:51Z","message":{"model":"<synthetic>","role":"assistant",` + text + `}}`,
		"the text alone":      `{"type":"assistant","uuid":"4","timestamp":"2026-09-01T07:08:51Z","message":{"role":"assistant",` + text + `}}`,
	} {
		ev, err := ParseLine([]byte(line))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !ev.APIError {
			t.Errorf("%s: not read as an API error", name)
		}
		if ev.Status != 403 {
			t.Errorf("%s: status %d, want 403", name, ev.Status)
		}
	}

	// The model's own words are never mistaken for the gateway's.
	for name, line := range map[string]string{
		"prose about errors": `{"type":"assistant","uuid":"5","timestamp":"2026-09-01T07:08:51Z","message":{"role":"assistant","content":[{"type":"text","text":"The API error: 403 in the logs was the token, not the quota."}]}}`,
		"a user turn":        `{"type":"user","uuid":"6","timestamp":"2026-09-01T07:08:51Z","message":{"role":"user","content":"API Error: 403 — that is what I saw, what now?"}}`,
	} {
		ev, err := ParseLine([]byte(line))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if ev.APIError {
			t.Errorf("%s: read as an API error", name)
		}
	}
}
