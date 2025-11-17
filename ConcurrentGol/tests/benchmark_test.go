package tests

// concurrency

import (
	"fmt"
	"os"
	"testing"

	"uk.ac.bris.cs/gameoflife/gol"
)

var benchCases = []gol.Params{
	{ImageWidth: 16, ImageHeight: 16},
	{ImageWidth: 64, ImageHeight: 64},
	{ImageWidth: 512, ImageHeight: 512},
}

var benchTurns = []int{1, 50, 100}

func BenchmarkGol(b *testing.B) {
	oldStdout := os.Stdout
	null, _ := os.Open(os.DevNull)
	defer func() {
		os.Stdout = oldStdout
		null.Close()
	}()
	os.Stdout = null

	for _, base := range benchCases {
		for _, turns := range benchTurns {
			for threads := 1; threads <= 16; threads++ {
				p := base
				p.Turns = turns
				p.Threads = threads

				name := fmt.Sprintf("%dx%dx%d-%d", p.ImageWidth, p.ImageHeight, p.Turns, p.Threads)

				b.Run(name, func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						events := make(chan gol.Event)
						go gol.Run(p, events, nil)

						// consume all the events until FinalTurnComplete
						for range events {
						}
					}
				})
			}
		}
	}
}

// go test ./tests -run ^$ -bench . -benchtime 1x -count 6 | tee results.out
// go run golang.org/x/perf/cmd/benchstat -format csv results.out | tee results.csv
