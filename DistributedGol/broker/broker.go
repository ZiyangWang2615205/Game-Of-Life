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

// for single AWS node broker was just send it back to the request
// it it still has same engine with the distributor.go
// // but it internally split the board by stripte then send it to worker nodes and proceed halo exchange.

type Broker struct {
	// connected worker list (each for one AWS node)
	workers []*rpc.Client

	// current world/turn info. (AliveCellsCount, SaveCurrent, Paused)
	mu       sync.Mutex
	curWorld [][]uint8
	curTurn  int
	paused   bool
	running  bool
	cond     *sync.Cond

	// snapshot when paused
	snapWorld [][]uint8
	snapTurn  int
}

// deepCopyWorld creates a deep copy of a [][]uint8 world.
// using for stablised return the snapshot from Savecurrent
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

// dialWorkers takes a comma-separated list of addresses and creates RPC connections to all workers
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

// partitionRows divides H rows into nearly equal parts for the workers.
func partitionRows(H, numWorkers int) [][2]int {
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

// ExecuteGol receives the full world from the controller, sends each turn's stripe + halo to the workers, and gathers their results
func (b *Broker) ExecuteGol(req stubs.Request, res *stubs.Response) error { // [NEW]
	b.mu.Lock()
	b.running = false // signal the previous execution loop to stop
	if b.cond != nil {
		b.cond.Broadcast()
	}
	b.mu.Unlock()

	// Explicitly reset the state
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

		// fan-out: send each worker its stripe plus halo
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
				// empty stripe (when workers > rows) – just skip it
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

			go func(i, s, e int, c *rpc.Client, rq stubs.StripeRequest) {
				var r stubs.StripeResponse
				err := c.Call(stubs.WorkerStep, rq, &r)
				ch <- stripeResult{idx: i, start: s, end: e, rows: r.NewStripe, alive: r.AliveCount, callErr: err}
			}(i, s, e, cli, reqStripe)
		}

		// fan-in: gather stripe results to build the new world
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
			aliveTotal += resStripe.alive
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

// AliveCellsCount counts the live cells in the broker’s current world and returns the result as an event
func (b *Broker) AliveCellsCount(_ stubs.Request, res *stubs.Response) error {
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

// SaveCurrent returns the current world as is
func (b *Broker) SaveCurrent(_ stubs.Request, res *stubs.Response) error {
	b.mu.Lock()
	var snap [][]uint8
	var turn int

	if b.paused && b.snapWorld != nil {
		// If paused, save using the snapshot from right before the pause
		snap = deepCopyWorld(b.snapWorld)
		turn = b.snapTurn
	} else {
		// Otherwise, save based on the current curWorld
		snap = deepCopyWorld(b.curWorld)
		turn = b.curTurn
	}

	b.mu.Unlock()

	// Fill the Response outside the lock
	res.NewWorld = snap
	res.Turn = turn
	return nil
}

// ShutDown stops the main loop and can notify workers to shut down if needed
func (b *Broker) ShutDown(_ stubs.Request, res *stubs.Response) error {
	b.mu.Lock()
	snap := deepCopyWorld(b.curWorld)
	turn := b.curTurn

	b.running = false
	if b.cond != nil {
		b.cond.Broadcast()
	}
	b.mu.Unlock()

	// let worker knows the shutdown
	for _, cli := range b.workers {
		_ = cli.Call(stubs.WorkerShutdown, struct{}{}, &struct{}{})
	}

	// fill the result world
	res.NewWorld = snap
	res.Turn = turn
	return nil
}

// Paused toggles the pause flag
func (b *Broker) Paused(_ stubs.Request, res *stubs.Response) error { // [NEW]
	b.mu.Lock()
	defer b.mu.Unlock()

	//b.paused = !b.paused
	if !b.paused {
		// Entering the paused state
		b.paused = true

		// Save the world/turn snapshot from just before the pause
		b.snapWorld = deepCopyWorld(b.curWorld)
		b.snapTurn = b.curTurn
	} else {
		// exit the paused state
		b.paused = false
		if b.cond != nil {
			b.cond.Broadcast()
		}
	}

	res.IsPaused = b.paused
	res.NewWorld = b.curWorld
	res.Turn = b.curTurn
	return nil
}

func main() {
	// Supports multiple workers while staying compatible with the original single-worker flag
	single := flag.String("worker", "", "single worker engine address (backwards compatible)")
	workersCSV := flag.String("workers", "", "comma-separated worker engine addresses")
	port := flag.String("port", "8030", "Broker listen port")
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

	if err := rpc.RegisterName("Engine", broker); err != nil {
		log.Fatal("failed to register broker as Engine:", err)
	}

	ln, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		log.Fatal("Broker listen error:", err)
	}
	defer ln.Close()

	log.Printf("Broker started on %s, workers: %v\n", *port, addrs)
	rpc.Accept(ln)
}

// go run broker.go -port 8030 -workers "172.31.73.131:8031, 172.31.64.52:8031, 172.31.78.137:8031, 172.31.69.135:8031"
// those 4 ip should be private
