package terminal

import (
	"bytes"
	"testing"
)

func TestSessionOutputBufferKeepsTail(t *testing.T) {
	session := &Session{bufferLimit: 8}
	session.appendOutput([]byte("12345"))
	session.appendOutput([]byte("67890"))
	if !bytes.Equal(session.buffer, []byte("34567890")) {
		t.Fatalf("unexpected buffer %q", session.buffer)
	}
	session.appendOutput([]byte("abcdefghijk"))
	if !bytes.Equal(session.buffer, []byte("defghijk")) {
		t.Fatalf("unexpected large buffer %q", session.buffer)
	}
}
