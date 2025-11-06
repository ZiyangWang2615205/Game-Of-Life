package main

import (
	"flag"
	"net"
	"net/rpc"
	"sync"

	"uk.ac.bris.cs/gameoflife/stubs"
)

// currentWorld used for check alive cells nums
var currentWorld [][]uint8

// currentTurn used for check current turn
var currentTurn int
var mu sync.Mutex

// run used for judge if server should run
var run bool
var paused bool

// ------------------------- 신규: 계산 정지/재개를 위한 조건변수 -------------------------
var cond = sync.NewCond(&mu)

//------------------------- 신규 끝 ----------------------------------------------------

// calcAliveNeighbours counts the number of alive neighbours
func calcAliveNeighbours(x, y int, world [][]uint8, height, width int) int {
	count := 0
	offestX := [3]int{-1, 0, 1}
	offestY := [3]int{-1, 0, 1}
	for _, dy := range offestY {
		for _, dx := range offestX {
			//count except itself
			if dy == 0 && dx == 0 {
				continue
			}
			//calculate neighbour coordinate
			nY := (y + dy + height) % height
			nX := (x + dx + width) % width
			if world[nY][nX] == 255 {
				count++
			}
		}
	}
	return count
}

type Engine struct{}

func (e *Engine) AliveCellsCount(req stubs.Request, res *stubs.Response) error {
	mu.Lock()
	defer mu.Unlock()
	count := 0
	for row := 0; row < len(currentWorld); row++ {
		for col := 0; col < len(currentWorld[0]); col++ {
			if currentWorld[row][col] == 255 {
				count++
			}
		}
	}

	res.AliveCells = count
	res.Turn = currentTurn
	return nil
}

func (e *Engine) ExecuteGol(req stubs.Request, res *stubs.Response) error {
	// gain the param
	mu.Lock()
	world := req.World
	turns := req.Turn
	height := req.ImageHeight
	width := req.ImageWidth
	paused = false
	run = true
	currentWorld = world
	currentTurn = 0
	mu.Unlock()

	// 기존 보조 고루틴은 pause에 실질 영향이 없음 → 제거
	// go func() {
	// 	for t := 0; t < turns; t++ {
	// 		mu.Lock()
	// 		if !run {
	// 			mu.Unlock()
	// 			return
	// 		}
	// 		if paused {
	// 			mu.Lock()
	// 			time.Sleep(200 * time.Millisecond)
	// 		}
	// 	}
	// }()

	turn := 0
	for turn < turns {
		//------------------------- 신규: 여기서 pause를 실제로 블록 -------------------------
		mu.Lock()
		for paused {
			cond.Wait() // Resume 시 Broadcast로 깨어남
		}
		if !run { // k로 종료된 경우 등
			res.NewWorld = world
			mu.Unlock()
			return nil
		}
		mu.Unlock()
		//------------------------- 신규 끝 ------------------------------------------------

		//create new world to store next state
		newWorld := make([][]uint8, height)
		for i := range newWorld {
			newWorld[i] = make([]uint8, width)
		}

		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				aliveNeighbours := calcAliveNeighbours(x, y, world, height, width)
				//live cell
				if world[y][x] == 255 {
					//any live cell with fewer than two/ more than three live neighbours dies
					if aliveNeighbours < 2 || aliveNeighbours > 3 {
						newWorld[y][x] = 0
					} else {
						//any live cell with more than three live neighbours dies
						newWorld[y][x] = 255

					}
				}

				//die cell
				if world[y][x] == 0 {
					if aliveNeighbours == 3 {
						//any dead cell with exactly three live neighbours becomes alive
						newWorld[y][x] = 255
					}
				}
			}
		}

		//updates world
		world = newWorld
		turn++

		mu.Lock()
		currentTurn = turn
		currentWorld = world
		mu.Unlock()
	}
	//send result to response pointer
	res.NewWorld = world
	return nil
}

// func (e *Engine) SaveCurrent(req stubs.Request, res stubs.Response) error {
func (e *Engine) SaveCurrent(req stubs.Request, res *stubs.Response) error { // ------------------------- 포인터로 수정
	mu.Lock()
	res.NewWorld = currentWorld
	res.Turn = currentTurn
	mu.Unlock()
	return nil
}

// func (e *Engine) ShutDown(req stubs.Request, res stubs.Response) error {
func (e *Engine) ShutDown(req stubs.Request, res *stubs.Response) error { // ------------------------- 포인터로 수정
	mu.Lock()
	run = false
	cond.Broadcast() // ------------------------- 신규: 대기 중 루프 깨우기
	res.NewWorld = currentWorld
	res.Turn = currentTurn
	mu.Unlock()
	return nil
}

// func (e *Engine) Paused(req stubs.Request, res stubs.Response) error {
func (e *Engine) Paused(req stubs.Request, res *stubs.Response) error { // ------------------------- 토글 + 포인터 수정
	mu.Lock()
	paused = !paused
	if !paused {
		cond.Broadcast() // 재개
	}
	res.IsPaused = paused
	res.NewWorld = currentWorld
	res.Turn = currentTurn
	mu.Unlock()
	return nil
}

func main() {
	//gain the port
	pAddr := flag.String("port", "8030", "Port to listen on")
	flag.Parse()
	rpc.Register(&Engine{})
	ln, _ := net.Listen("tcp", ":"+*pAddr)
	defer ln.Close()
	rpc.Accept(ln)
}
