package main

import "testing"

func TestReplaceTag(t *testing.T) {
	in := "<environment_context>\n  <current_date>2026-09-23</current_date>\n  <timezone>Asia/Shanghai</timezone>\n</environment_context>"
	out := replaceTag(replaceTag(in, "timezone", "America/New_York"), "current_date", "2026-09-22")
	want := "<environment_context>\n  <current_date>2026-09-22</current_date>\n  <timezone>America/New_York</timezone>\n</environment_context>"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
	esc := `\u003ctimezone\u003eAsia/Shanghai\u003c/timezone\u003e`
	got := replaceTag(esc, "timezone", "America/New_York")
	wantEsc := `\u003ctimezone\u003eAmerica/New_York\u003c/timezone\u003e`
	if got != wantEsc {
		t.Fatalf("esc got %q", got)
	}
}
