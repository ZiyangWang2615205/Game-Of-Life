package gol

import (
	"fmt"
	"log"
	"net/rpc"

	"uk.ac.bris.cs/gameoflife/stubs"
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

// saveCurWorld is used to save current world
func saveCurWorld(p Params, c distributorChannels, world [][]uint8, turn int) {
	filename := fmt.Sprintf("%dx%dx%d", p.ImageWidth, p.ImageHeight, turn)
	//output the new graph
	c.ioCommand <- ioOutput
	c.ioFilename <- fmt.Sprintf(filename)
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			c.ioOutput <- world[y][x]
		}
	}

	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- ImageOutputComplete{
		CompletedTurns: turn,
		Filename:       filename,
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels, keyPresses <-chan rune) {
	//connect with AWS server
	server := "3.88.113.240:8030"
	client, err := rpc.Dial("tcp", server)
	if err != nil {
		log.Fatal("Dialing: ", err)
	}
	defer client.Close()

	// TODO: Create a 2D slice to store the world.
	world := make([][]uint8, p.ImageHeight)
	for i := range world {
		world[i] = make([]uint8, p.ImageWidth)
	}

	//make Io read the init graph,filename can be 16x16, 64x64 etc.
	c.ioCommand <- ioInput
	c.ioFilename <- fmt.Sprintf("%dx%d", p.ImageWidth, p.ImageHeight)

	//receive pixel from Io
	for i := 0; i < p.ImageHeight; i++ {
		for j := 0; j < p.ImageWidth; j++ {
			world[i][j] = <-c.ioInput
		}
	}

	turn := 0
	c.events <- StateChange{turn, Executing}

	//create RPC
	req := stubs.Request{
		World:       world,
		Turn:        p.Turns,
		ImageHeight: p.ImageHeight,
		ImageWidth:  p.ImageWidth,
	}

	var res *stubs.Response = &stubs.Response{}

	//RPC ExecuteGol
	err = client.Call(stubs.EngineStart, req, res)
	if err != nil {
		log.Fatal("fail to use RPC: ", err)
	}

	//receive result and updates world
	world = res.NewWorld

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
