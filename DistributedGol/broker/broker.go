package main

import (
	"flag"
	"net"
	"net/rpc"
	"sync"

	"uk.ac.bris.cs/gameoflife/stubs"
)

// partitionRows used for divide world to several rows
func partitionRows(height, num int) [][]int {
	chunk := height / num
	var parts [][]int
	//assign rows
	for i := 0; i < num; i++ {
		startY := i * chunk
		endY := startY + chunk
		if i == num-1 {
			endY = height
		}
		parts = append(parts, []int{startY, endY})
	}
	return parts
}

// because only broker has the latest whole world, it should be response to send num of alive cells

// aliveCount used for calculate number of alive cells right now
func aliveCount(world [][]uint8) int {
	count := 0
	for y := 0; y < len(world); y++ {
		for x := 0; x < len(world[0]); x++ {
			if world[y][x] == 255 {
				count++
			}
		}
	}
	return count
}

// each port represent one worker
var defaultWorkers = []string{
	"localhost:8031",
	"localhost:8032",
	"localhost:8033",
	"localhost:8034",
}

// wcli used for label worker and help broker RPC
type wcli struct {
	client  *rpc.Client
	address string
}

type workerRes struct {
	startY int
	endY   int
	rowRes [][]uint8
	err    error
}

type Broker struct {
	mu   sync.Mutex
	cond *sync.Cond

	//worker list
	worker []wcli
	parts  [][]int

	world  [][]uint8
	width  int
	height int
	//turns are total turn
	turns int
	turn  int

	run    bool
	paused bool
	done   bool
	err    error
}

func (b *Broker) fail(err error) {
	b.mu.Lock()
	b.err = err
	b.done = true
	b.mu.Unlock()
}

func (b *Broker) runLoop(req stubs.BrokerRequest) {
	addrs := req.Workers

	if len(addrs) == 0 {
		addrs = defaultWorkers
	}

	//create workers
	wc := make([]wcli, len(addrs))
	for i, addr := range addrs {
		cli, err := rpc.Dial("tcp", addr)
		if err != nil {
			b.fail(err)
			return
		}
		wc[i] = wcli{
			client:  cli,
			address: addr,
		}
	}

	//gain jobs
	parts := partitionRows(b.height, len(wc))

	b.mu.Lock()
	b.worker = wc
	b.parts = parts
	b.mu.Unlock()

	for {
		b.mu.Lock()
		//if game finish we break program
		if b.done || b.turn >= b.turns {
			b.mu.Unlock()
			break
		}

		for b.paused {
			b.cond.Wait()
		}

		//gain init status
		world := b.world
		height := b.height
		width := b.width
		b.mu.Unlock()

		var wg sync.WaitGroup
		res := make([]workerRes, len(wc))
		//use error channel to deal with all error of go routine
		errCh := make(chan error, len(wc))

		for w := range wc {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				startY, endY := parts[w][0], parts[w][1]

				var output stubs.WorkerResponse
				err := wc[w].client.Call(stubs.WorkerStep, stubs.WorkerRequest{
					World:  world,
					StartY: startY,
					EndY:   endY,
					Width:  width,
					Height: height,
				}, &output)

				if err != nil {
					errCh <- err
					return
				}

				res[w] = workerRes{
					startY: startY,
					endY:   endY,
					rowRes: output.RowRes,
					err:    err,
				}
			}(w)
		}

		wg.Wait()

		//deal with error
		select {
		case err := <-errCh:
			b.fail(err)
			break
		default:
		}

		newWorld := make([][]uint8, b.height)
		for _, r := range res {
			if r.err != nil || len(r.rowRes) == 0 {
				continue
			}
			for y := 0; y < r.endY-r.startY; y++ {
				newWorld[r.startY+y] = r.rowRes[y]
			}
		}

		//updates world
		b.mu.Lock()
		b.world = newWorld
		b.turn++
		b.mu.Unlock()
	}

	//close all worker when task ending
	b.mu.Lock()
	b.done = true
	for _, w := range b.worker {
		_ = w.client.Close()
	}
	b.mu.Unlock()
}

func (b *Broker) Start(req stubs.BrokerRequest, res *stubs.BrokerResponse) error {
	b.mu.Lock()
	if b.run {
		b.mu.Unlock()
		return nil
	}

	//initial param
	b.world = req.World
	b.turns = req.Turns
	b.width = req.ImageWidth
	b.height = req.ImageHeight
	b.turn = 0
	b.paused = false
	b.done = false
	b.err = nil
	b.run = true
	b.mu.Unlock()

	b.cond = sync.NewCond(&b.mu)
	go b.runLoop(req)
	return nil
}

func (b *Broker) SaveCurrent(req stubs.BrokerRequest, res *stubs.BrokerResponse) error {
	b.mu.Lock()
	res.World = b.world
	b.mu.Unlock()
	return nil
}

func (b *Broker) Pause(req stubs.BrokerRequest, res *stubs.BrokerResponse) error {
	b.mu.Lock()
	if b.paused {
		b.paused = false
		b.cond.Broadcast()
	} else {
		b.paused = true
	}
	res.World = b.world
	b.mu.Unlock()
	return nil
}

func (b *Broker) Shutdown(req stubs.BrokerRequest, res *stubs.BrokerResponse) error {
	b.mu.Lock()
	b.done = true
	b.cond.Broadcast()
	res.World = b.world
	for _, w := range b.worker {
		_ = w.client.Close()
	}
	b.mu.Unlock()
	return nil
}

func (b *Broker) AliveCellsCount(req stubs.BrokerRequest, res *stubs.BrokerResponse) error {
	b.mu.Lock()
	res.World = b.world
	res.Turn = b.turn
	res.AliveCells = aliveCount(b.world)
	b.mu.Unlock()
	return nil
}

func main() {
	pAddr := flag.String("port", "8050", "Port to listen on")
	flag.Parse()
	rpc.Register(&Broker{})
	ln, _ := net.Listen("tcp", ":"+*pAddr)
	defer ln.Close()
	rpc.Accept(ln)
}
