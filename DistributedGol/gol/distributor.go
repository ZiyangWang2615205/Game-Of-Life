package gol

import (
	"fmt"
	"log"
	"net/rpc"
	"time"

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
	server := "98.93.248.212:8030"
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

	//RPC ExecuteGol
	exe := make(chan struct{})
	go func() {
		var executeRes stubs.Response
		_ = client.Call(stubs.EngineStart, stubs.Request{
			World:       world,
			Turn:        p.Turns,
			ImageHeight: p.ImageHeight,
			ImageWidth:  p.ImageWidth,
		}, &executeRes)
		close(exe)
	}()

	//RPC AliveCellsCount
	ticker := time.NewTicker(2 * time.Second)
	done := make(chan bool)
	go func() {
		for {
			select {
			case <-ticker.C:
				var aliveRes stubs.Response
				err = client.Call(stubs.EngineCount, stubs.Request{}, &aliveRes)
				if err != nil {
					log.Fatal("fail to use AliveCellsCount: ", err)
				}
				//send the AliveCellsCount event
				c.events <- AliveCellsCount{
					CompletedTurns: aliveRes.Turn,
					CellsCount:     aliveRes.AliveCells,
				}

			case <-done:
				ticker.Stop()
				return
			}
		}
	}()

	pause := false
	for {
		select {
		case key := <-keyPresses:
			switch key {
			case 's':
				//If s is pressed, the controller should generate a PGM file with the current state of the board.
				var saveRes stubs.Response
				err := client.Call(stubs.EngineSave, stubs.Request{}, &saveRes)
				if err != nil {
					log.Fatal("fail to use SaveCurrent: ", err)
				}
				saveCurWorld(p, c, saveRes.NewWorld, saveRes.Turn)

			case 'q':
				//If q is pressed, close the controller client program without causing an error on the Gol server.
				done <- true
				// Make sure that the Io has finished any output before exiting.
				c.ioCommand <- ioCheckIdle
				<-c.ioIdle

				c.events <- StateChange{turn, Quitting}

				// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
				close(c.events)

			case 'k':
				//If k is pressed, all components of the distributed system are shut down cleanly, and the system outputs a PGM image of the latest state.
				var overRes stubs.Response
				_ = client.Call(stubs.EngineOver, stubs.Request{}, &overRes)
				saveCurWorld(p, c, overRes.NewWorld, overRes.Turn)

				done <- true
				c.ioCommand <- ioCheckIdle
				<-c.ioIdle

				c.events <- StateChange{overRes.Turn, Quitting}
				close(c.events)

			case 'p':
				//If p is pressed, pause the processing on the AWS node and have the controller print the current turn that is being processed.
				var pauseRes stubs.Response
				if !pause {
					client.Call(stubs.EnginePaused, stubs.Request{}, &pauseRes)
					fmt.Printf("Turn %d is being processed\n", pauseRes.Turn)
					c.events <- StateChange{pauseRes.Turn, Paused}
					pause = true
				} else {
					//If p is pressed again resume the processing and have the controller print Continuing.
					client.Call(stubs.EngineResumed, stubs.Request{}, &stubs.Response{})
					fmt.Println("Continuing")
					c.events <- StateChange{pauseRes.Turn, Executing}
				}
			}
		case <-exe:
			var res stubs.Response
			client.Call(stubs.EngineSave, stubs.Request{}, &res)
			//stop the time ticker
			done <- true
			saveCurWorld(p, c, res.NewWorld, res.Turn)
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
	}
}
