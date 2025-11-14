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
//   - 노드 수: 환경변수 GOL_NODES 로 설정 (기본 1)
//   - 벤치마크 이름: Gol/16x16x1-<nodes>-<GOMAXPROCS>
//     -> benchstat 결과 CSV 에서 name 컬럼이 "Gol/..." 로 시작하게 되어 plot.py 가 그대로 파싱 가능.
func BenchmarkGol(b *testing.B) {
	// stdout 눌러서 벤치마크 출력 안 섞이게
	oldStdout := os.Stdout
	null, _ := os.Open(os.DevNull)
	defer func() {
		os.Stdout = oldStdout
		null.Close()
	}()
	os.Stdout = null

	// 1) 노드 수 결정: GOL_NODES 환경변수 (없으면 1)
	nodes := 1
	if s := os.Getenv("GOL_NODES"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			nodes = v
		}
	}

	// 2) 현재 GOMAXPROCS 값 (CPU 개수) 기록해서 이름에 넣음
	cpus := runtime.GOMAXPROCS(0)

	for _, base := range benchCases {
		for _, turns := range benchTurns {
			p := base
			p.Turns = turns
			// Threads 필드는 원래 로컬 워커 스레드 수였지만,
			// 지금 구조에서는 AWS 노드 수를 의미하도록 맞춰서 기록해 둠.
			p.Threads = nodes

			// name 예: "16x16x1-1-8"  (1노드, GOMAXPROCS=8)
			// benchstat 이름은 "Gol/16x16x1-1-8" 형태가 되고,
			// plot.py 의 정규식이 그대로 동작한다.
			name := fmt.Sprintf("%dx%dx%d-%d-%d", p.ImageWidth, p.ImageHeight, p.Turns, nodes, cpus)

			b.Run(name, func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					events := make(chan gol.Event)
					// keyPresses 는 벤치마크에서 사용하지 않으므로 nil
					go gol.Run(p, events, nil)

					// FinalTurnComplete 나올 때까지 이벤트 전부 소비
					for range events {
					}
				}
			})
		}
	}
}

// 벤치마크 실행 예시:
//
// 1노드
//   GOL_NODES=1 go test ./tests -run ^$ -bench . -benchtime 1x -count 6 | tee results_1node.out
//
// 2노드
//   GOL_NODES=2 go test ./tests -run ^$ -bench . -benchtime 1x -count 6 | tee results_2node.out
//
// 3노드
//   GOL_NODES=3 go test ./tests -run ^$ -bench . -benchtime 1x -count 6 | tee results_3node.out
//
// 4노드
//   GOL_NODES=4 go test ./tests -run ^$ -bench . -benchtime 1x -count 6 | tee results_4node.out

//
// go run golang.org/x/perf/cmd/benchstat -format csv \
//   results_1node.out \
//   results_2node.out \
//   results_3node.out \
//   results_4node.out \
//   > results.csv

// 이런 식으로 CSV 를 만든 다음 plot.py 를 실행하면 된다.
