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

var height, width, totalTurns int

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

func (e *Engine) Initialise(req stubs.Request, res *stubs.Response) error {
	mu.Lock()
	defer mu.Unlock()

	//initialise
	currentWorld = req.World
	currentTurn = 0
	totalTurns = req.Turn
	height = req.ImageHeight
	width = req.ImageWidth
	paused = false
	run = true

	//return the initialise state
	res.NewWorld = currentWorld
	res.Turn = currentTurn
	return nil
}

func (e *Engine) ExecuteGol(req stubs.Request, res *stubs.Response) error {
	mu.Lock()
	defer mu.Unlock()

	//press 'k'
	if !run {
		res.NewWorld = currentWorld
		res.Turn = currentTurn
		return nil
	}

	//execution end
	if currentTurn >= totalTurns {
		run = false
		res.NewWorld = currentWorld
		res.Turn = currentTurn
		return nil
	}

	//press 'p'
	if paused {
		res.NewWorld = currentWorld
		res.Turn = currentTurn
		return nil
	}

	//deal with gol logic
	newWorld := make([][]uint8, height)
	for i := range newWorld {
		newWorld[i] = make([]uint8, width)
	}

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			aliveNeighbours := calcAliveNeighbours(x, y, currentWorld, height, width)
			//live cell
			if currentWorld[y][x] == 255 {
				//any live cell with fewer than two/ more than three live neighbours dies
				if aliveNeighbours < 2 || aliveNeighbours > 3 {
					newWorld[y][x] = 0
				} else {
					//any live cell with more than three live neighbours dies
					newWorld[y][x] = 255

				}
			}

			//die cell
			if currentWorld[y][x] == 0 {
				if aliveNeighbours == 3 {
					//any dead cell with exactly three live neighbours becomes alive
					newWorld[y][x] = 255
				}
			}
		}
	}

	//updates world
	currentWorld = newWorld
	currentTurn++
	res.NewWorld = currentWorld
	res.Turn = currentTurn
	return nil
}

func (e *Engine) SaveCurrent(req stubs.Request, res *stubs.Response) error {
	mu.Lock()
	defer mu.Unlock()
	res.NewWorld = currentWorld
	res.Turn = currentTurn
	return nil
}

func (e *Engine) ShutDown(req stubs.Request, res *stubs.Response) error {
	mu.Lock()
	defer mu.Unlock()
	run = false
	res.NewWorld = currentWorld
	res.Turn = currentTurn
	return nil
}

func (e *Engine) Paused(req stubs.Request, res *stubs.Response) error {
	mu.Lock()
	defer mu.Unlock()
	paused = true
	res.Turn = currentTurn
	return nil
}

func (e *Engine) Resumed(req stubs.Request, res *stubs.Response) error {
	mu.Lock()
	defer mu.Unlock()
	paused = false
	res.Turn = currentTurn
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
