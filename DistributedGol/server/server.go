package main

import (
	"flag"
	"net"
	"net/rpc"

	"uk.ac.bris.cs/gameoflife/stubs"
)

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

func (e *Engine) ExecuteGol(req stubs.Request, res *stubs.Response) error {
	// gain the param
	world := req.World
	turns := req.Turn
	height := req.ImageHeight
	width := req.ImageWidth

	turn := 0
	for turn < turns {
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
	}
	//send result to response pointer
	res.NewWorld = world
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
