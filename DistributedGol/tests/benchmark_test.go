package tests

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"testing"

	"uk.ac.bris.cs/gameoflife/gol"
)

var benchCases = []gol.Params{
	{ImageWidth: 16, ImageHeight: 16},
	{ImageWidth: 64, ImageHeight: 64},
	{ImageWidth: 512, ImageHeight: 512},
}

var benchTurns = []int{1, 50, 100}

// BenchmarkGol
//   - node : number of node is basically 1
//   - name of benchmark: Gol/16x16x1-<nodes>-<GOMAXPROCS>
func BenchmarkGol(b *testing.B) {
	// stdout, make the benchmark result stay in order
	oldStdout := os.Stdout
	null, _ := os.Open(os.DevNull)
	defer func() {
		os.Stdout = oldStdout
		null.Close()
	}()
	os.Stdout = null

	// decide number of nodes: GOL_NODES envirionment variables (default 1)
	nodes := 1
	if s := os.Getenv("GOL_NODES"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			nodes = v
		}
	}

	// current GOMAXPROCS value
	cpus := runtime.GOMAXPROCS(0)

	for _, base := range benchCases {
		for _, turns := range benchTurns {
			p := base
			p.Turns = turns

			p.Threads = nodes

			name := fmt.Sprintf("%dx%dx%d-%d-%d", p.ImageWidth, p.ImageHeight, p.Turns, nodes, cpus)

			b.Run(name, func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					events := make(chan gol.Event)
					// keyPresses is not using in benchmark thus nil
					go gol.Run(p, events, nil)

					// consume all FinalTurnComplete until show up
					for range events {
					}
				}
			})
		}
	}
}

// benchmark execution example:
//
// 1node
//   GOL_NODES=1 go test ./tests -run ^$ -bench . -benchtime 1x -count 6 | tee results_1node.out
//
// 2node
//   GOL_NODES=2 go test ./tests -run ^$ -bench . -benchtime 1x -count 6 | tee results_2node.out
//
// 3node
//   GOL_NODES=3 go test ./tests -run ^$ -bench . -benchtime 1x -count 6 | tee results_3node.out
//
// 4node
//   GOL_NODES=4 go test ./tests -run ^$ -bench . -benchtime 1x -count 6 | tee results_4node.out

//
// go run golang.org/x/perf/cmd/benchstat -format csv \
//   results_1node.out \
//   results_2node.out \
//   results_3node.out \
//   results_4node.out \
//   > results.csv
