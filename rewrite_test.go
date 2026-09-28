package main

import "testing"

func TestReplaceTag(t *testing.T) {
	in := "<environment_context>\n  <current_date>2026-09-28</current_date>\n  <timezone>America/New_York</timezone>\n</environment_context>"
	got := replaceTag(replaceTag(in, "timezone", defaultTZ), "current_date", "2026-09-22")
	want := "<environment_context>\n  <current_date>2026-09-22</current_date>\n  <timezone>Asia/Singapore</timezone>\n</environment_context>"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	esc := `\u003ctimezone\u003eAsia/Shanghai\u003c/timezone\u003e`
	got = replaceTag(esc, "timezone", defaultTZ)
	want = `\u003ctimezone\u003eAsia/Singapore\u003c/timezone\u003e`
	if got != want {
		t.Fatalf("escaped got %q want %q", got, want)
	}
}
