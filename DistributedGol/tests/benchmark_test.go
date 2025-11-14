package tests

// distributed benchmark (client → broker → workers)

import (
	"fmt"
	"os"
	"testing"

	"uk.ac.bris.cs/gameoflife/gol"
)

// 벤치마크에서 사용할 보드 크기 조합들
var benchCases = []gol.Params{
	{ImageWidth: 16, ImageHeight: 16},
	{ImageWidth: 64, ImageHeight: 64},
	{ImageWidth: 512, ImageHeight: 512},
}

// 사용할 턴 수 조합
var benchTurns = []int{1, 50, 100}

// 분산 버전 전체 파이프라인에 대한 벤치마크
// 주의: 이 벤치마크를 돌릴 때는 broker + worker 서버들이 이미 떠 있어야 함.
func BenchmarkGol(b *testing.B) {
	// stdout 무력화 (서버/클라이언트 로그가 많으면 벤치마크가 오염되므로)
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

				// benchstat / plot.py에서 파싱하기 위한 이름 형식:
				// Gol/512x512x100-8-8
				name := fmt.Sprintf("Gol/%dx%dx%d-%d-%d",
					p.ImageWidth, p.ImageHeight, p.Turns, p.Threads, 8) // 마지막 8은 CPU logical cores label

				b.Run(name, func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						events := make(chan gol.Event)
						go gol.Run(p, events, nil)

						// FinalTurnComplete까지 이벤트 소비
						for range events {
						}
					}
				})
			}
		}
	}
}

// 실행 예시:
//   go test ./tests -run ^$ -bench . -benchtime 1x -count 6 | tee results.out
//   go test ./tests -run ^$ -bench . -benchtime 1x -count 6 -timeout 60m | tee results.out
//   go run golang.org/x/perf/cmd/benchstat -format csv results.out | tee results.csv
