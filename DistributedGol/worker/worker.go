package main

import (
	"flag"
	"net"
	"net/rpc"

	"uk.ac.bris.cs/gameoflife/stubs"
)

type Worker struct{}

func countAlive(slice [][]uint8, aboveRow, belowRow []uint8, y, x, width int) int {
	h := len(slice)
	alive := 0

	// ----------------------------
	// 上一行（用 aboveRow 或 slice[y-1]）
	// ----------------------------
	var up []uint8
	if y == 0 {
		up = aboveRow
	} else {
		up = slice[y-1]
	}

	if up[(x-1+width)%width] == 255 {
		alive++
	}
	if up[x] == 255 {
		alive++
	}
	if up[(x+1)%width] == 255 {
		alive++
	}

	// ----------------------------
	// 下一行（用 belowRow 或 slice[y+1]）
	// ----------------------------
	var down []uint8
	if y == h-1 {
		down = belowRow
	} else {
		down = slice[y+1]
	}

	if down[(x-1+width)%width] == 255 {
		alive++
	}
	if down[x] == 255 {
		alive++
	}
	if down[(x+1)%width] == 255 {
		alive++
	}

	// ----------------------------
	// 左右邻居（同一行）
	// ----------------------------
	if slice[y][(x-1+width)%width] == 255 {
		alive++
	}
	if slice[y][(x+1)%width] == 255 {
		alive++
	}

	return alive
}

func computeNextSlice(slice [][]uint8, aboveRow, belowRow []uint8, width int) [][]uint8 {
	height := len(slice)
	// init next state slice
	newSlice := make([][]uint8, height)
	for i := range newSlice {
		newSlice[i] = make([]uint8, width)
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			alive := countAlive(slice, aboveRow, belowRow, y, x, width)

			//deal with logic of gol
			if slice[y][x] == 255 {
				if alive == 2 || alive == 3 {
					newSlice[y][x] = 255
				} else {
					newSlice[y][x] = 0
				}
			} else {
				if alive == 3 {
					newSlice[y][x] = 255
				} else {
					newSlice[y][x] = 0
				}
			}
		}
	}
	return newSlice
}

func (w *Worker) Compute(req stubs.SliceRequest, res *stubs.SliceResponse) error {
	res.NewSlice = computeNextSlice(req.Slice, req.AboveRow, req.BelowRow, req.ImageWidth)
	return nil
}

func main() {
	port := flag.String("port", "8030", "Port to listen on")
	flag.Parse()

	rpc.RegisterName("Worker", &Worker{})

	ln, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		panic(err)
	}
	defer ln.Close()

	rpc.Accept(ln)
}
