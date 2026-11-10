//go:build prof

// The prof build (go build -tags prof) writes profiles. Release binaries do not have it.
//
//	GODOIST_CPUPROF=file     a CPU profile of the run
//	GODOIST_TRACE=file       an execution trace of the run (go tool trace)
//	GODOIST_GOROUTINES=file  the goroutines at the end of the run
//
// SIGUSR1 adds the goroutine count and stacks to /tmp/godoist-goroutines.txt.

package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"syscall"
)

func init() {
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGUSR1)
		for range ch {
			f, err := os.OpenFile("/tmp/godoist-goroutines.txt", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				continue
			}
			fmt.Fprintf(f, "=== %d goroutines\n", runtime.NumGoroutine())
			pprof.Lookup("goroutine").WriteTo(f, 1)
			f.Close()
		}
	}()
	// stop runs the stop functions in the reverse order of their start.
	var stops []func()
	if p := os.Getenv("GODOIST_CPUPROF"); p != "" {
		if f, err := os.Create(p); err == nil && pprof.StartCPUProfile(f) == nil {
			stops = append(stops, func() {
				pprof.StopCPUProfile()
				f.Close()
			})
		}
	}
	if p := os.Getenv("GODOIST_TRACE"); p != "" {
		if f, err := os.Create(p); err == nil && trace.Start(f) == nil {
			stops = append(stops, func() {
				trace.Stop()
				f.Close()
			})
		}
	}
	if p := os.Getenv("GODOIST_GOROUTINES"); p != "" {
		stops = append(stops, func() {
			if f, err := os.Create(p); err == nil {
				pprof.Lookup("goroutine").WriteTo(f, 1)
				f.Close()
			}
		})
	}
	stopProf = func() {
		for i := len(stops) - 1; i >= 0; i-- {
			stops[i]()
		}
	}
}
