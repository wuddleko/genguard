package check

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestTailRingKeepsLast50Lines(t *testing.T) {
	t.Parallel()
	var ring tailRing
	var written strings.Builder
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&written, "l%d\n", i)
	}
	n, err := ring.Write([]byte(written.String()))
	if err != nil || n != written.Len() {
		t.Fatalf("Write = %d, %v", n, err)
	}

	var want strings.Builder
	for i := 10; i < 60; i++ {
		fmt.Fprintf(&want, "l%d\n", i)
	}
	if got := ring.String(); got != want.String() {
		t.Fatalf("tail = %q, want %q", got, want.String())
	}
}

func TestTailRingCutsLongLine(t *testing.T) {
	t.Parallel()
	var ring tailRing
	over := bytes.Repeat([]byte("x"), commandTailLineBytes+100)
	payload := append(append([]byte{}, over...), []byte("\nshort\n")...)
	n, err := ring.Write(payload)
	if err != nil || n != len(payload) {
		t.Fatalf("Write = %d, %v", n, err)
	}
	want := strings.Repeat("x", commandTailLineBytes) + "\nshort\n"
	if got := ring.String(); got != want {
		t.Fatalf("tail mismatch at %d (len %d, want %d)", firstDiff(got, want), len(got), len(want))
	}

	var exact tailRing
	line := bytes.Repeat([]byte("y"), commandTailLineBytes)
	payload = append(append([]byte{}, line...), []byte("\nnext\n")...)
	n, err = exact.Write(payload)
	if err != nil || n != len(payload) {
		t.Fatalf("Write = %d, %v", n, err)
	}
	want = string(line) + "\nnext\n"
	if got := exact.String(); got != want {
		t.Fatalf("exact tail mismatch at %d (len %d, want %d)", firstDiff(got, want), len(got), len(want))
	}
}

func firstDiff(got, want string) int {
	n := len(got)
	if len(want) < n {
		n = len(want)
	}
	for i := 0; i < n; i++ {
		if got[i] != want[i] {
			return i
		}
	}
	return n
}

func TestTailRingStripsCRLF(t *testing.T) {
	t.Parallel()
	var ring tailRing
	payload := []byte("one\r\ntwo\r\n")
	n, err := ring.Write(payload)
	if err != nil || n != len(payload) {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if got := ring.String(); got != "one\ntwo\n" {
		t.Fatalf("tail = %q", got)
	}
}

func TestTailRingTrailingNewline(t *testing.T) {
	t.Parallel()
	var ring tailRing
	if _, err := ring.Write([]byte("one\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := ring.Write([]byte("two")); err != nil {
		t.Fatal(err)
	}
	if _, err := ring.Write([]byte("\n")); err != nil {
		t.Fatal(err)
	}
	if got := ring.String(); got != "one\ntwo\n" {
		t.Fatalf("tail = %q", got)
	}
}

func TestTailRingSuccessLeavesTailUnread(t *testing.T) {
	t.Parallel()
	var ring tailRing
	if got := ring.String(); got != "" {
		t.Fatalf("empty tail = %q", got)
	}
	payload := []byte("generated files\n")
	n, err := ring.Write(payload)
	if err != nil || n != len(payload) {
		t.Fatalf("Write = %d, %v", n, err)
	}
}
