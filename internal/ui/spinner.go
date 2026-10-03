package ui

import (
	"fmt"
	"time"
)

func Spinner() (stop func()) {
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		for i := 0; ; i++ {
			fmt.Printf("\r%s thinking...", frames[i%len(frames)])
			select {
			case <-done:
				fmt.Print("\r\033[K")
				return
			case <-ticker.C:
			}

		}
	}()
	return func() {
		close(done)
		<-finished
	}
}
