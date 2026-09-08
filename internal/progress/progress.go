package progress

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	sharedterminal "github.com/roshbhatia/go-utils/terminal"
)

type Indicator struct {
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

func Start(output io.Writer, label string, enabled bool) *Indicator {
	indicator := &Indicator{stop: make(chan struct{}), done: make(chan struct{})}
	file, terminal := output.(*os.File)
	if !enabled || !terminal || !sharedterminal.IsTTY(file) {
		close(indicator.done)
		return indicator
	}
	go func() {
		defer close(indicator.done)
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		index := 0
		for {
			select {
			case <-indicator.stop:
				_, _ = fmt.Fprint(output, "\r\x1b[2K")
				return
			case <-ticker.C:
				_, _ = fmt.Fprintf(output, "\r%s %s", frames[index%len(frames)], label)
				index++
			}
		}
	}()
	return indicator
}

func (indicator *Indicator) Stop() {
	indicator.once.Do(func() { close(indicator.stop) })
	<-indicator.done
}
