package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/backup"
	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/restore"
)

func TestExitCode(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{nil, 0},
		{fmt.Errorf("--namespace is required"), 1},
		{&partialError{failed: 3}, 2},
		{fmt.Errorf("wrapped: %w", &partialError{failed: 1}), 2},
	}
	for _, c := range cases {
		if got := exitCode(c.err); got != c.want {
			t.Errorf("exitCode(%v) = %d, want %d", c.err, got, c.want)
		}
	}
}

func TestReportRestore_FailuresReturnPartialError(t *testing.T) {
	var buf bytes.Buffer
	err := reportRestore(&buf, false, &restore.Result{Created: 2, Failed: 2, Errors: []string{"a", "b"}})
	var p *partialError
	if !errors.As(err, &p) || p.failed != 2 {
		t.Fatalf("err = %v, want partialError with 2 failed", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("Failed:   2")) {
		t.Errorf("text output missing failed count:\n%s", buf.String())
	}
}

func TestReportRestore_SuccessReturnsNil(t *testing.T) {
	var buf bytes.Buffer
	if err := reportRestore(&buf, false, &restore.Result{Created: 4, Retargeted: 3}); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("Retargeted references: 3")) {
		t.Errorf("text output missing retargeted count:\n%s", buf.String())
	}
}

func TestReportRestore_JSON(t *testing.T) {
	var buf bytes.Buffer
	err := reportRestore(&buf, true, &restore.Result{Created: 1, Skipped: 1, Failed: 1, Retargeted: 2, Errors: []string{"boom"}})
	if exitCode(err) != 2 {
		t.Errorf("exit code = %d, want 2", exitCode(err))
	}
	var got map[string]any
	if jerr := json.Unmarshal(buf.Bytes(), &got); jerr != nil {
		t.Fatalf("output is not one JSON object: %v\n%s", jerr, buf.String())
	}
	want := map[string]any{"ok": false, "created": 1.0, "updated": 0.0, "skipped": 1.0, "failed": 1.0, "retargeted": 2.0}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	if errs, _ := got["errors"].([]any); len(errs) != 1 {
		t.Errorf("errors = %v, want 1 entry", got["errors"])
	}
	if w, ok := got["warnings"].([]any); !ok || len(w) != 0 {
		t.Errorf("warnings = %v, want empty list", got["warnings"])
	}
}

func TestReportBackup_ErrorsReturnPartialError(t *testing.T) {
	var buf bytes.Buffer
	err := reportBackup(&buf, true, "out", &backup.Result{ObjectCount: 3, Errors: []string{"get x: 500"}})
	if exitCode(err) != 2 {
		t.Errorf("exit code = %d, want 2", exitCode(err))
	}
	var got map[string]any
	if jerr := json.Unmarshal(buf.Bytes(), &got); jerr != nil {
		t.Fatalf("output is not one JSON object: %v\n%s", jerr, buf.String())
	}
	if got["ok"] != false || got["object_count"] != 3.0 || got["output_dir"] != "out" {
		t.Errorf("unexpected report: %v", got)
	}
}

func TestReportBackup_CleanReturnsNil(t *testing.T) {
	var buf bytes.Buffer
	if err := reportBackup(&buf, true, "out", &backup.Result{ObjectCount: 3}); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	var got map[string]any
	_ = json.Unmarshal(buf.Bytes(), &got)
	if got["ok"] != true {
		t.Errorf("ok = %v, want true", got["ok"])
	}
}
