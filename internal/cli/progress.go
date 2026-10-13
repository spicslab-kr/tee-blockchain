package cli

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Green wraps s in ANSI green when stdout is a terminal and NO_COLOR is not set.
func Green(s string) string {
	if !colorStdout() {
		return s
	}
	return "\x1b[32m" + s + "\x1b[0m"
}

// Okln prints a line to stdout, green on a terminal — for the final "it worked" line.
func Okln(a ...any) {
	fmt.Println(Green(fmt.Sprint(a...)))
}

// Task is one step in progress. Start prints "message"; on a terminal it then adds dots (. .. ...) to show it is
// waiting, and Done/Fail finish the same line as "message... ✓ result" or "message... ✗ reason". When output is piped
// (tests, scripts) there is no animation, only the single line "message... ✓ result".
type Task struct {
	msg  string
	tty  bool
	stop chan struct{}
	wg   sync.WaitGroup
	once sync.Once
}

// Start begins a step. Call Done or Fail exactly once.
func Start(msg string) *Task {
	t := &Task{msg: msg, tty: colorStdout(), stop: make(chan struct{})}
	if !t.tty {
		fmt.Print(msg + "...")
		return t
	}
	fmt.Print(msg)
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		tick := time.NewTicker(350 * time.Millisecond)
		defer tick.Stop()
		n := 0
		for {
			select {
			case <-t.stop:
				return
			case <-tick.C:
				n = n%3 + 1
				fmt.Printf("\r%s%s%s", msg, strings.Repeat(".", n), strings.Repeat(" ", 3-n))
			}
		}
	}()
	return t
}

func (t *Task) end(mark, detail string) {
	t.once.Do(func() {
		if t.tty {
			close(t.stop)
			t.wg.Wait()
			fmt.Printf("\r%s... ", t.msg)
		} else {
			fmt.Print(" ")
		}
		if detail != "" {
			fmt.Println(mark + " " + detail)
		} else {
			fmt.Println(mark)
		}
	})
}

// Done closes the step as a success: "msg... ✓ detail".
func (t *Task) Done(detail string) { t.end(Green("✓"), detail) }

// Fail closes the step as a failure: "msg... ✗ detail" (red).
func (t *Task) Fail(detail string) { t.end(Red("✗"), detail) }

// Why prints one sentence on why the step that just finished is needed, as an indented line, so readers who do not
// know our terms can still follow.
func Why(s string) {
	fmt.Println("      · " + s)
}

func colorStdout() bool {
	if _, set := os.LookupEnv("NO_COLOR"); set {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
