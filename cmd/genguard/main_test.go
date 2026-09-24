package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/cli"
)

func TestRunSetsVersion(t *testing.T) {
	previous := cli.Version
	t.Cleanup(func() { cli.Version = previous })
	cli.Version = "replaced"

	oldOut, oldErr := os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	os.Stderr = w
	code := run([]string{"version"})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = oldOut
	os.Stderr = oldErr
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if buf.String() != version+"\n" {
		t.Fatalf("stdout = %q, want %q", buf.String(), version+"\n")
	}
	if cli.Version != version {
		t.Fatalf("cli.Version = %q, want %q", cli.Version, version)
	}
	if strings.TrimSpace(version) == "" {
		t.Fatal("version is empty")
	}
}

func TestMainExitsWithRunStatus(t *testing.T) {
	var got int
	called := false
	prev := osExit
	osExit = func(code int) {
		called = true
		got = code
	}
	t.Cleanup(func() { osExit = prev })

	oldArgs := os.Args
	os.Args = []string{"genguard", "version"}
	t.Cleanup(func() { os.Args = oldArgs })

	oldOut, oldErr := os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	os.Stderr = w
	main()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = oldOut
	os.Stderr = oldErr
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if !called || got != 0 {
		t.Fatalf("osExit called=%v code=%d", called, got)
	}
	if buf.String() != version+"\n" {
		t.Fatalf("stdout = %q", buf.String())
	}
}
