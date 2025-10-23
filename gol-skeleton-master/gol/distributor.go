package gol

import (
	"fmt"
	"time"

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

// calcAliveNeighbours counts the number of alive neighbours
func calcAliveNeighbours(x, y int, world [][]uint8, p Params) int {
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

	//workJob type helps us to distribute job
	type workerJob struct {
		startY int
		endY   int
		world  [][]uint8
	}

	//workerRes type helps to return the res to distributor
	type workerRes struct {
		startY int
		rowRes [][]uint8
	}

	//create channel
	jobs := make(chan workerJob, p.Threads)
	res := make(chan workerRes, p.Threads)

	//create time ticker and reflect alive cells each 2s
	ticker := time.NewTicker(2 * time.Second)
	done := make(chan bool)

	go func(p Params) {
		for {
			select {
			case <-ticker.C:
				count := 0
				//calc number of alive cells
				for y := 0; y < p.ImageHeight; y++ {
					for x := 0; x < p.ImageWidth; x++ {
						if world[y][x] == 255 {
							count++
						}
					}
				}

				c.events <- AliveCellsCount{
					CompletedTurns: turn,
					CellsCount:     count,
				}

			case <-done:
				ticker.Stop()
				return
			}
		}
	}(p)
	//start workers
	for i := 0; i < p.Threads; i++ {
		go func() {
			//receive the job from jobs channel
			for job := range jobs {
				startY := job.startY
				endY := job.endY
				//tell worker which area belongs to him
				workerArea := make([][]uint8, endY-startY)
				for index := range workerArea {
					workerArea[index] = make([]uint8, p.ImageWidth)
				}
				//do the job
				for y := startY; y < endY; y++ {
					for x := 0; x < p.ImageWidth; x++ {
						aliveNeighbours := calcAliveNeighbours(x, y, job.world, p)
						//judge stage
						//0-die, 255-alive
						if job.world[y][x] == 255 {
							if aliveNeighbours < 2 || aliveNeighbours > 3 {
								//any live cell with fewer than two live neighbours dies
								//any live cell with more than three live neighbours dies
								workerArea[y-startY][x] = 0
							} else {
								//any live cell with two or three live neighbours is unaffected
								workerArea[y-startY][x] = 255
							}
						}

						if job.world[y][x] == 0 {
							//any dead cell with exactly three live neighbours becomes alive
							if aliveNeighbours == 3 {
								workerArea[y-startY][x] = 255
							} else {
								workerArea[y-startY][x] = 0
							}
						}
					}
				}
				//res channel to receive the return of each worker
				res <- workerRes{
					startY: startY,
					rowRes: workerArea,
				}
			}

		}()
	}
	// TODO: Execute all turns of the Game of Life.
	for turn < p.Turns {
		//record next state
		newWorld := make([][]uint8, p.ImageHeight)
		for i := range newWorld {
			newWorld[i] = make([]uint8, p.ImageWidth)
		}

		//distribute the job to thread
		chunk := p.ImageHeight / p.Threads
		for i := 0; i < p.Threads; i++ {
			//init start and end
			startY := i * chunk
			endY := startY + chunk
			//ensure if the last part could not be divided equally
			if i == p.Threads-1 {
				endY = p.ImageHeight
			}
			jobs <- workerJob{
				startY: startY,
				endY:   endY,
				world:  world,
			}
		}

		//receive the result from res channel
		for i := 0; i < p.Threads; i++ {
			finalRes := <-res
			for j, row := range finalRes.rowRes {
				newWorld[finalRes.startY+j] = row
			}
		}
		//updates world
		world = newWorld
		turn++
	}

	//ensure time ticker stop
	done <- true

	//output the new graph
	c.ioCommand <- ioOutput
	c.ioFilename <- fmt.Sprintf("%dx%dx%d", p.ImageWidth, p.ImageHeight, p.Turns)
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
