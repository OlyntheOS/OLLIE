package stream

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestFramerRoundTrip(t *testing.T) {
	buf := &bytes.Buffer{}
	framer := NewFramer(buf, buf, 1024)
	payload := []byte("hello")

	if err := framer.WriteFrame(payload); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	got, err := framer.ReadFrame()
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("frame mismatch: got %q want %q", got, payload)
	}
}

func TestFramerRejectsOversize(t *testing.T) {
	framer := NewFramer(bytes.NewBuffer(nil), io.Discard, 4)
	if err := framer.WriteFrame([]byte("12345")); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("expected ErrFrameTooLarge, got %v", err)
	}
}
