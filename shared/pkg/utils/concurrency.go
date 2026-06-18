package utils

import (
	"log/slog"
)

// SafeGo runs a function inside a background goroutine, catching and logging any panics
// to prevent the entire web server process from crashing.
func SafeGo(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Panic recovered in background goroutine", "panic", r)
			}
		}()
		fn()
	}()
}
