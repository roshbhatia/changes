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
	type result struct {
		output []byte
		err    error
	}
	results := make(chan result, 1)
	go func() {
		var output bytes.Buffer
		buffer := make([]byte, 4096)
		for {
			read, readErr := terminal.Read(buffer)
			if read > 0 {
				_, _ = output.Write(buffer[:read])
			}
			if bytes.Contains(output.Bytes(), []byte("reading changes")) && bytes.Contains(output.Bytes(), []byte("\x1b[2K")) {
				results <- result{output: output.Bytes()}
				return
			}
			if readErr != nil {
				results <- result{output: output.Bytes(), err: readErr}
				return
			}
		}
	}()
	indicator := Start(peer, "reading changes", true)
	time.Sleep(100 * time.Millisecond)
	indicator.Stop()
	select {
	case read := <-results:
		if read.err != nil {
			t.Fatalf("read terminal: %v; output = %q", read.err, read.output)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out reading terminal animation and clear sequence")
	}
}
