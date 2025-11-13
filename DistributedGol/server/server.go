package main

import (
	"flag"
	"log"
	"net"
	"net/rpc"

	"uk.ac.bris.cs/gameoflife/stubs"
)

// ===== NEW: stateless worker node for multi-node Game of Life =====
//
// 이 파일은 원래 단일 노드 엔진(Engine)을 제공하던 서버 대신,
// 브로커가 호출하는 Worker RPC 서비스를 제공합니다.
// 컨트롤러(distributor.go)는 여전히 브로커만 'Engine'으로 보고,
// 개별 워커와는 직접 통신하지 않습니다.

type Worker struct{} // [NEW]

// countNeighbours는 주어진 셀의 8-이웃 생존 셀 수를 계산합니다.
// Stripe 내부와 위/아래 halo 행을 함께 사용합니다.
func countNeighbours(x, y int, stripe [][]uint8, width int, haloTop, haloBottom []uint8) int { // [NEW]
	h := len(stripe)
	count := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			nx := (x + dx + width) % width
			ny := y + dy

			var v uint8
			switch {
			case ny < 0:
				v = haloTop[nx]
			case ny >= h:
				v = haloBottom[nx]
			default:
				v = stripe[ny][nx]
			}
			if v == 255 {
				count++
			}
		}
	}
	return count
}

// Step는 하나의 stripe에 대해 Game of Life를 한 턴 진행합니다.
func (w *Worker) Step(req stubs.StripeRequest, res *stubs.StripeResponse) error { // [NEW]
	stripe := req.Stripe
	h := req.LocalH
	wth := req.ImageWidth

	newStripe := make([][]uint8, h)
	for y := 0; y < h; y++ {
		newStripe[y] = make([]uint8, wth)
	}

	alive := 0
	for y := 0; y < h; y++ {
		for x := 0; x < wth; x++ {
			neigh := countNeighbours(x, y, stripe, wth, req.HaloTop, req.HaloBottom)
			cur := stripe[y][x]
			if cur == 255 {
				if neigh == 2 || neigh == 3 {
					newStripe[y][x] = 255
					alive++
				} else {
					newStripe[y][x] = 0
				}
			} else {
				if neigh == 3 {
					newStripe[y][x] = 255
					alive++
				} else {
					newStripe[y][x] = 0
				}
			}
		}
	}

	res.NewStripe = newStripe
	res.AliveCount = alive
	return nil
}

// Ping / Shutdown은 브로커가 워커 상태를 확인·종료할 때 사용 가능합니다.
// 과제 필수는 아니지만 인터페이스 확장을 위해 추가했습니다.
func (w *Worker) Ping(_ struct{}, _ *struct{}) error { // [NEW]
	return nil
}

func (w *Worker) Shutdown(_ struct{}, _ *struct{}) error { // [NEW]
	// 실제 종료는 main의 Listen 루프를 끊는 식으로 구현할 수도 있지만,
	// 과제에서는 단순 no-op으로 두어도 무방합니다.
	return nil
}

func main() {
	pAddr := flag.String("port", "8031", "Port to listen on") // [NEW] 기본 포트를 워커용으로 분리
	flag.Parse()

	if err := rpc.RegisterName("Worker", &Worker{}); err != nil { // [NEW]
		log.Fatal("failed to register Worker:", err)
	}

	ln, err := net.Listen("tcp", ":"+*pAddr) // [NEW]
	if err != nil {
		log.Fatal("worker listen error:", err)
	}
	defer ln.Close()

	log.Printf("Worker listening on %s\n", *pAddr) // [NEW]
	rpc.Accept(ln)
}
