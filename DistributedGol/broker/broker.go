package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"strings"
	"sync"

	"uk.ac.bris.cs/gameoflife/stubs"
)

// ===== NEW: Broker now implements Engine and fans out to multiple workers =====
//
// 기존 브로커는 단일 AWS 노드(Engine)에 모든 요청을 그대로 전달하는 역할만 했습니다.
// 아래 Broker는 컨트롤러(distributor.go) 입장에서는 여전히 'Engine'으로 보이지만,
// 내부적으로 여러 Worker 노드에 보드를 stripe로 나눠 전달하고 halo exchange를 수행합니다.

type Broker struct {
	// [NEW] 연결된 워커 리스트 (각각 하나의 AWS 노드)
	workers []*rpc.Client

	// [NEW] 현재 월드/턴 정보 (AliveCellsCount, SaveCurrent, Paused 지원용)
	mu       sync.Mutex
	curWorld [][]uint8
	curTurn  int
	paused   bool
	running  bool
	cond     *sync.Cond
}

// deepCopyWorld creates a deep copy of a [][]uint8 world.
// SaveCurrent에서 스냅샷을 안정적으로 돌려주기 위해 사용한다.
func deepCopyWorld(src [][]uint8) [][]uint8 { // 음....
	h := len(src)
	if h == 0 {
		return nil
	}
	dst := make([][]uint8, h)
	for y := 0; y < h; y++ {
		row := src[y]
		if row == nil {
			continue
		}
		w := len(row)
		dst[y] = make([]uint8, w)
		copy(dst[y], row)
	}
	return dst
}

// dialWorkers는 콤마로 구분된 주소 목록을 받아 모든 워커에 RPC 연결을 맺습니다.
func dialWorkers(addrs []string) ([]*rpc.Client, error) { // [NEW]
	clients := make([]*rpc.Client, 0, len(addrs))
	for _, a := range addrs {
		addr := strings.TrimSpace(a)
		if addr == "" {
			continue
		}
		c, err := rpc.Dial("tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("dial worker %s: %w", addr, err)
		}
		clients = append(clients, c)
	}
	if len(clients) == 0 {
		return nil, fmt.Errorf("no valid worker addresses")
	}
	return clients, nil
}

// partitionRows는 H개의 행을 워커 수만큼 거의 균등하게 나눕니다.
func partitionRows(H, numWorkers int) [][2]int { // [NEW]
	stripes := make([][2]int, numWorkers)
	base := H / numWorkers
	extra := H % numWorkers
	offset := 0
	for i := 0; i < numWorkers; i++ {
		h := base
		if i < extra {
			h++
		}
		stripes[i] = [2]int{offset, offset + h} // [start, end)
		offset += h
	}
	return stripes
}

// ExecuteGol은 컨트롤러로부터 전체 월드를 받아,
// 매 턴마다 워커들에게 stripe + halo를 보내고 결과를 수집합니다.
func (b *Broker) ExecuteGol(req stubs.Request, res *stubs.Response) error { // [NEW]
	b.mu.Lock()
	b.running = false // 이전 실행 루프에게 종료 신호
	if b.cond != nil {
		b.cond.Broadcast()
	}
	b.mu.Unlock()

	// 잠깐 기다릴지, 아니면 바로 덮어쓸지는 과제에서 자유지만,
	// 최소한 상태를 명시적으로 reset:
	b.mu.Lock()
	b.curWorld = req.World
	b.curTurn = 0
	b.paused = false
	b.running = true
	if b.cond == nil {
		b.cond = sync.NewCond(&b.mu)
	}
	b.mu.Unlock()

	H, W := req.ImageHeight, req.ImageWidth
	if H == 0 || W == 0 {
		res.NewWorld = req.World
		res.Turn = 0
		return nil
	}

	if len(b.workers) == 0 {
		return fmt.Errorf("no workers connected")
	}

	world := req.World

	b.mu.Lock()
	b.curWorld = world
	b.curTurn = 0
	b.running = true
	if b.cond == nil {
		b.cond = sync.NewCond(&b.mu)
	}
	b.mu.Unlock()

	stripes := partitionRows(H, len(b.workers))

	for t := 0; t < req.Turn; t++ {
		b.mu.Lock()
		for b.paused {
			b.cond.Wait()
		}
		if !b.running {
			b.mu.Unlock()
			break
		}
		b.mu.Unlock()

		// fan-out: 각 워커에 stripe + halo 전송
		type stripeResult struct {
			idx     int
			start   int
			end     int
			rows    [][]uint8
			alive   int
			callErr error
		}
		ch := make(chan stripeResult, len(b.workers))
		active := 0

		for i, cli := range b.workers {
			s, e := stripes[i][0], stripes[i][1]
			if s >= e {
				// 빈 stripe (워커 수 > 행 수인 경우) - 그냥 스킵
				continue
			}
			active++

			top := (s - 1 + H) % H
			bottom := e % H

			stripe := make([][]uint8, e-s)
			for r := s; r < e; r++ {
				stripe[r-s] = world[r]
			}

			reqStripe := stubs.StripeRequest{
				Stripe:     stripe,
				HaloTop:    world[top],
				HaloBottom: world[bottom],
				ImageWidth: W,
				LocalH:     e - s,
			}

			go func(i, s, e int, c *rpc.Client, rq stubs.StripeRequest) { // [NEW]
				var r stubs.StripeResponse
				err := c.Call(stubs.WorkerStep, rq, &r)
				ch <- stripeResult{idx: i, start: s, end: e, rows: r.NewStripe, alive: r.AliveCount, callErr: err}
			}(i, s, e, cli, reqStripe)
		}

		// fan-in: stripe 결과 모아 새 world 구성
		next := make([][]uint8, H)
		aliveTotal := 0
		completed := 0
		for completed < active {
			resStripe := <-ch
			if resStripe.callErr != nil {
				return fmt.Errorf("worker %d call error: %w", resStripe.idx, resStripe.callErr)
			}
			if resStripe.rows == nil {
				completed++
				continue
			}
			for y := resStripe.start; y < resStripe.end; y++ {
				rowIdx := y - resStripe.start
				if next[y] == nil {
					next[y] = make([]uint8, W)
				}
				copy(next[y], resStripe.rows[rowIdx])
			}
			aliveTotal += resStripe.alive // (원하면 여기서 통계용으로 사용 가능)
			completed++
		}

		world = next

		b.mu.Lock()
		b.curWorld = world
		b.curTurn++
		b.mu.Unlock()
	}

	res.NewWorld = world
	res.Turn = b.curTurn
	return nil
}

// AliveCellsCount는 브로커가 들고 있는 최신 월드를 기준으로
// 살아 있는 셀 개수를 센 뒤 이벤트로 돌려줍니다.
func (b *Broker) AliveCellsCount(_ stubs.Request, res *stubs.Response) error { // [NEW]
	b.mu.Lock()
	defer b.mu.Unlock()

	alive := 0
	for y := range b.curWorld {
		for x := range b.curWorld[y] {
			if b.curWorld[y][x] == 255 {
				alive++
			}
		}
	}
	res.AliveCells = alive
	res.Turn = b.curTurn
	return nil
}

// SaveCurrent는 현재 월드를 그대로 반환합니다.
func (b *Broker) SaveCurrent(_ stubs.Request, res *stubs.Response) error {
	b.mu.Lock()
	// curWorld, curTurn의 일관된 스냅샷 확보
	snap := deepCopyWorld(b.curWorld)
	turn := b.curTurn
	b.mu.Unlock()

	// 락 밖에서 Response 채우기
	res.NewWorld = snap
	res.Turn = turn
	return nil
}

// ShutDown은 메인 루프를 멈추고, 필요하다면 워커에게도 종료를 알릴 수 있습니다.
func (b *Broker) ShutDown(_ stubs.Request, res *stubs.Response) error { // [NEW]
	b.mu.Lock()
	b.running = false
	if b.cond != nil {
		b.cond.Broadcast()
	}
	b.mu.Unlock()

	// 선택: 워커에게도 Shutdown RPC를 보낼 수 있음 (필수 아님)
	for _, cli := range b.workers {
		_ = cli.Call(stubs.WorkerShutdown, struct{}{}, &struct{}{})
	}

	res.Turn = b.curTurn
	return nil
}

// Paused는 일시정지 플래그를 토글합니다.
func (b *Broker) Paused(_ stubs.Request, res *stubs.Response) error { // [NEW]
	b.mu.Lock()
	defer b.mu.Unlock()

	b.paused = !b.paused
	if !b.paused && b.cond != nil {
		b.cond.Broadcast()
	}

	res.IsPaused = b.paused
	res.NewWorld = b.curWorld
	res.Turn = b.curTurn
	return nil
}

func main() {
	// 기존 단일 워커 플래그와의 호환성을 유지하면서, 다중 워커도 지원합니다.
	single := flag.String("worker", "", "single worker engine address (backwards compatible)") // [NEW]
	workersCSV := flag.String("workers", "", "comma-separated worker engine addresses")        // [NEW]
	port := flag.String("port", "8030", "Broker listen port")                                  // [NEW]
	flag.Parse()

	var addrs []string
	if *workersCSV != "" {
		addrs = strings.Split(*workersCSV, ",")
	} else if *single != "" {
		addrs = []string{*single}
	} else {
		log.Fatal("no workers specified: use -worker or -workers")
	}

	clients, err := dialWorkers(addrs)
	if err != nil {
		log.Fatal("failed to connect workers:", err)
	}

	broker := &Broker{workers: clients}
	broker.cond = sync.NewCond(&broker.mu)

	if err := rpc.RegisterName("Engine", broker); err != nil { // [NEW]
		log.Fatal("failed to register broker as Engine:", err)
	}

	ln, err := net.Listen("tcp", ":"+*port) // [NEW]
	if err != nil {
		log.Fatal("Broker listen error:", err)
	}
	defer ln.Close()

	log.Printf("Broker started on %s, workers: %v\n", *port, addrs) // [NEW]
	rpc.Accept(ln)
}
