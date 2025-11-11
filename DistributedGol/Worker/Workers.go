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

func calcResRow(world [][]uint8, startY, endY, height, width int) [][]uint8 {
	resRow := make([][]uint8, endY-startY)
	for i := range resRow {
		resRow[i] = make([]uint8, width)
	}

	for y := startY; y < endY; y++ {
		for x := 0; x < width; x++ {
			aliveNeighbours := calcAliveNeighbours(x, y, world, height, width)
			if world[y][x] == 255 {
				if aliveNeighbours == 2 || aliveNeighbours == 3 {
					resRow[y-startY][x] = 255
				} else {
					resRow[y-startY][x] = 0
				}
			} else {
				if aliveNeighbours == 3 {
					resRow[y-startY][x] = 255
				} else {
					resRow[y-startY][x] = 0
				}
			}
		}
	}
	return resRow
}

type Worker struct{}

// Step used to receive one row result from one worker
func (w *Worker) Step(req stubs.WorkerRequest, res *stubs.WorkerResponse) error {
	res.RowRes = calcResRow(req.World, req.StartY, req.EndY, req.Height, req.Width)
	return nil
}

func main() {
	pAddr := flag.String("port", "8031", "Port to listen on")
	flag.Parse()
	rpc.Register(&Worker{})
	ln, _ := net.Listen("tcp", ":"+*pAddr)
	defer ln.Close()
	rpc.Accept(ln)
}
