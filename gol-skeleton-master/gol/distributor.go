package gol

import (
	"fmt"

	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	// TODO: Create a 2D slice to store the world.
	world := make([][]uint8, p.ImageHeight)
	for i := range world {
		world[i] = make([]uint8, p.ImageWidth)
	}

	turn := 0
	c.events <- StateChange{turn, Executing}

	//make Io read the init graph,filename can be 16x16, 64x64 etc.
	c.ioCommand <- ioInput
	c.ioFilename <- fmt.Sprintf("%dx%d", p.ImageWidth, p.ImageHeight)

	//receive pixel from Io
	for i := 0; i < p.ImageHeight; i++ {
		for j := 0; j < p.ImageWidth; j++ {
			world[i][j] = <-c.ioInput
		}
	}

	// TODO: Execute all turns of the Game of Life.
	for turn < p.Turns {
		//record next state
		newWorld := make([][]uint8, p.ImageHeight)
		for i := range newWorld {
			newWorld[i] = make([]uint8, p.ImageWidth)
		}

		//transverse cells and judge state by rule
		for y := 0; y < p.ImageHeight; y++ {
			for x := 0; x < p.ImageWidth; x++ {
				//use aux to calculate alive neighbours
				aliveNeighbours := func(x, y int, world [][]uint8, p Params) int {
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
							nY := (y + dy + p.ImageHeight) % p.ImageHeight
							nX := (x + dx + p.ImageWidth) % p.ImageWidth
							if world[nY][nX] == 255 {
								count++
							}
						}
					}
					return count
				}(x, y, world, p)

				//judge stage
				//0-die, 255-alive
				if world[y][x] == 255 {
					if aliveNeighbours < 2 {
						//any live cell with fewer than two live neighbours dies
						newWorld[y][x] = 0
					} else if aliveNeighbours <= 3 {
						//any live cell with two or three live neighbours is unaffected
						newWorld[y][x] = 255
					} else {
						//any live cell with more than three live neighbours dies
						newWorld[y][x] = 0
					}
				}

				if world[y][x] == 0 {
					//any dead cell with exactly three live neighbours becomes alive
					if aliveNeighbours == 3 {
						newWorld[y][x] = 255
					}
				}
			}
		}
		//updates world
		world = newWorld
		turn++
	}

	//output the new graph
	c.ioCommand <- ioOutput
	c.ioFilename <- fmt.Sprintf("%dx%d", p.ImageWidth, p.ImageHeight)
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			c.ioOutput <- world[y][x]
		}
	}

	// TODO: Report the final state using FinalTurnCompleteEvent.
	aliveCells := []util.Cell{}
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			if world[y][x] == 255 {
				aliveCells = append(aliveCells, util.Cell{
					X: x,
					Y: y,
				})
			}
		}
	}

	c.events <- FinalTurnComplete{CompletedTurns: p.Turns, Alive: aliveCells}

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}
