package main

import (
	"flag"
	"log"
	"net"
	"net/rpc"
	"strings"
	"sync"

	"uk.ac.bris.cs/gameoflife/stubs"
)

func splitWorld(world [][]uint8, n int) [][][]uint8 {
	h := len(world)
	if n > h {
		n = h
	}
	base := h / n
	rem := h % n
	parts := make([][][]uint8, n)
	row := 0
	for i := 0; i < n; i++ {
		size := base
		if i < rem {
			size++
		}
		parts[i] = world[row : row+size]
		row += size
	}
	return parts
}

func mergeParts(parts [][][]uint8) [][]uint8 {
	var world [][]uint8
	for _, p := range parts {
		world = append(world, p...)
	}
	return world
}

func getAboveRow(world [][]uint8, parts [][][]uint8, i int) []uint8 {
	// 当前块的第一行在 world 中的索引
	startIndex := 0
	for idx := 0; idx < i; idx++ {
		startIndex += len(parts[idx])
	}
	h := len(world)
	aboveIndex := (startIndex - 1 + h) % h
	return world[aboveIndex]
}

func getBelowRow(world [][]uint8, parts [][][]uint8, i int) []uint8 {
	startIndex := 0
	for idx := 0; idx <= i; idx++ {
		startIndex += len(parts[idx])
	}
	h := len(world)
	belowIndex := startIndex % h
	return world[belowIndex]
}

type Broker struct {
	mu      sync.Mutex
	cond    *sync.Cond
	world   [][]uint8
	turn    int
	paused  bool
	stop    bool
	height  int
	width   int
	workers []*rpc.Client
}

func (b *Broker) ExecuteGol(req stubs.Request, res *stubs.Response) error {
	b.mu.Lock()
	b.world = req.World
	b.turn = 0
	b.stop = false
	b.paused = false
	b.height = req.ImageHeight
	b.width = req.ImageWidth
	b.mu.Unlock()

	totalTurns := req.Turn

	// 初次切片
	parts := splitWorld(b.world, len(b.workers))

	for {
		b.mu.Lock()
		// 结束条件
		if b.turn >= totalTurns || b.stop {
			// 最后把结果填给 Response
			res.NewWorld = b.world
			res.Turn = b.turn
			b.mu.Unlock()
			return nil
		}

		// 暂停就等待
		for b.paused && !b.stop {
			b.cond.Wait()
		}
		if b.stop {
			res.NewWorld = b.world
			res.Turn = b.turn
			b.mu.Unlock()
			return nil
		}

		// 拿当前 world 快照用于本轮计算
		currentWorld := b.world
		b.mu.Unlock()

		// 并行调用每个 worker 算各自那一块
		newParts := make([][][]uint8, len(parts))
		var wg sync.WaitGroup
		wg.Add(len(parts))

		for i := range parts {
			i := i
			go func() {
				defer wg.Done()

				above := getAboveRow(currentWorld, parts, i)
				below := getBelowRow(currentWorld, parts, i)

				reqSlice := stubs.SliceRequest{
					Slice:      parts[i],
					AboveRow:   above,
					BelowRow:   below,
					ImageWidth: b.width,
				}
				var resp stubs.SliceResponse
				err := b.workers[i].Call(stubs.WorkerCompute, reqSlice, &resp)
				if err != nil {
					log.Printf("Worker %d error: %v\n", i, err)
					// 简单处理：如果失败就直接用原 slice
					newParts[i] = parts[i]
					return
				}
				newParts[i] = resp.NewSlice
			}()
		}

		wg.Wait()

		b.mu.Lock()
		// 更新 world 和 turn
		b.world = mergeParts(newParts)
		b.turn++
		// 下一轮继续用新的 parts
		parts = splitWorld(b.world, len(b.workers))
		b.mu.Unlock()
	}
}

// ================= Controller 调用的其他 RPC：Alive / Save / Pause / Over =================

func (b *Broker) AliveCellsCount(_ stubs.Request, res *stubs.Response) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	count := 0
	for y := range b.world {
		for x := range b.world[y] {
			if b.world[y][x] == 255 {
				count++
			}
		}
	}
	res.AliveCells = count
	res.Turn = b.turn
	return nil
}

func (b *Broker) SaveCurrent(_ stubs.Request, res *stubs.Response) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	res.NewWorld = b.world
	res.Turn = b.turn
	return nil
}

func (b *Broker) ShutDown(_ stubs.Request, res *stubs.Response) error {
	b.mu.Lock()
	b.stop = true
	b.cond.Broadcast()
	res.NewWorld = b.world
	res.Turn = b.turn
	b.mu.Unlock()
	return nil
}

func (b *Broker) Paused(_ stubs.Request, res *stubs.Response) error {
	b.mu.Lock()
	b.paused = !b.paused
	if !b.paused {
		b.cond.Broadcast()
	}
	res.IsPaused = b.paused
	res.Turn = b.turn
	res.NewWorld = b.world
	b.mu.Unlock()
	return nil
}

// ================= main：连接多个 worker，监听 controller =================

func main() {
	workersFlag := flag.String("workers", "127.0.0.1:8031,127.0.0.1:8032,127.0.0.1:8033", "comma separated worker addresses")
	port := flag.String("port", "8030", "Broker listen port (for controller)")
	flag.Parse()

	addrs := strings.Split(*workersFlag, ",")
	if len(addrs) == 0 {
		log.Fatal("No worker addresses provided")
	}

	var workerClients []*rpc.Client
	for _, addr := range addrs {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		client, err := rpc.Dial("tcp", addr)
		if err != nil {
			log.Fatalf("Failed to connect to worker %s: %v", addr, err)
		}
		log.Printf("Connected to worker %s\n", addr)
		workerClients = append(workerClients, client)
	}

	if len(workerClients) == 0 {
		log.Fatal("No valid workers connected")
	}

	b := &Broker{
		workers: workerClients,
	}
	b.cond = sync.NewCond(&b.mu)

	// 对 controller 暴露的名字还是 "Engine"，这样你的 distributor 完全不用改
	if err := rpc.RegisterName("Engine", b); err != nil {
		log.Fatal("Failed to register broker as Engine:", err)
	}

	ln, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		log.Fatal("Broker listen error:", err)
	}

	log.Printf("Broker started on %s, managing %d workers\n", *port, len(workerClients))

	rpc.Accept(ln)
}
