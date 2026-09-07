package progress

import (
	"bytes"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestIndicatorDoesNotWriteToRedirectedOutput(t *testing.T) {
	var output bytes.Buffer
	indicator := Start(&output, "reading changes", true)
	indicator.Stop()
	if output.Len() != 0 {
		t.Fatalf("redirected output = %q", output.String())
	}
}

func TestIndicatorAnimatesAndClearsATerminal(t *testing.T) {
	terminal, peer, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	defer peer.Close()
	indicator := Start(peer, "reading changes", true)
	time.Sleep(100 * time.Millisecond)
	indicator.Stop()
	output := make([]byte, 4096)
	read, _ := terminal.Read(output)
	output = output[:read]
	if !bytes.Contains(output, []byte("reading changes")) || !bytes.Contains(output, []byte("\x1b[2K")) {
		t.Fatalf("terminal output = %q", output)
	}
}
