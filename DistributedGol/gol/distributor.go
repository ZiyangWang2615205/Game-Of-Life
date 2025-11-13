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

func handleError(err error) {
	if err != nil {
		fmt.Println("there is a error :", err)
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels, keyPresses <-chan rune) {
	//connect with AWS server
	server := "54.91.38.226:8030"
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
					log.Printf("AliveCellsCount failed: %v", err)
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

	//RPC ExecuteGol
	type execResult struct {
		res stubs.Response
		err error // added error to help us deal with it more convenient
	}
	execDone := make(chan execResult, 1) // receive all error and result then deal with together
	go func() {
		var res stubs.Response
		err := client.Call(stubs.EngineStart, req, &res)
		execDone <- execResult{
			res: res,
			err: err,
		}
	}()

	fetchShot := func() ([][]uint8, int) {
		var res stubs.Response
		err := client.Call(stubs.EngineGetWorld, stubs.Request{}, &res)
		if err != nil {
			// if error output init graph
			return world, turn
		}
		return res.NewWorld, res.Turn
	}

	paused := false
	for {
		select {
		case key := <-keyPresses:
			switch key {
			case 's':
				shot, t := fetchShot()
				saveCurWorld(p, c, shot, t)
			case 'p':
				if !paused {
					var r stubs.Response
					err = client.Call(stubs.EnginePause, stubs.Request{}, &r)
					handleError(err)
					c.events <- StateChange{
						CompletedTurns: turn,
						NewState:       Paused,
					}
					paused = true
				} else {
					var r stubs.Response
					err = client.Call(stubs.EngineResume, stubs.Request{}, &r)
					handleError(err)
					c.events <- StateChange{
						CompletedTurns: 0, // while restart there are not completed turns
						NewState:       Executing,
					}
					paused = false
				}

			case 'q':
				shot, t := fetchShot()
				saveCurWorld(p, c, shot, t)
				// Make sure that the Io has finished any output before exiting.
				c.ioCommand <- ioCheckIdle
				<-c.ioIdle
				done <- true
				c.events <- StateChange{turn, Quitting}
				return

			case 'k':
				shot, t := fetchShot()
				saveCurWorld(p, c, shot, t)
				var r stubs.Response
				err = client.Call(stubs.EngineKill, stubs.Request{}, &r)
				handleError(err)
				done <- true
				// TODO: Report the final state using FinalTurnCompleteEvent.
				finalTurn := r.Turn
				finalWorld := shot
				if r.NewWorld != nil {
					finalWorld = r.NewWorld
				}

				aliveCells := []util.Cell{}
				for y := 0; y < p.ImageHeight; y++ {
					for x := 0; x < p.ImageWidth; x++ {
						if finalWorld[y][x] == 255 {
							aliveCells = append(aliveCells, util.Cell{
								X: x,
								Y: y,
							})
						}
					}
				}

				c.events <- FinalTurnComplete{CompletedTurns: finalTurn, Alive: aliveCells}
				// Make sure that the Io has finished any output before exiting.
				c.ioCommand <- ioCheckIdle
				<-c.ioIdle
				c.events <- StateChange{turn, Quitting}
				// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
				return
			}
		case r := <-execDone:
			//normal termination
			done <- true
			//don't forget to deal with error
			if r.err != nil {
				log.Fatal("ExecuteGol failed: ", r.err)
			}
			//updates world and turn
			world = r.res.NewWorld
			turn = r.res.Turn
			saveCurWorld(p, c, world, turn)
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

			c.events <- FinalTurnComplete{CompletedTurns: turn, Alive: aliveCells}
			// Make sure that the Io has finished any output before exiting.
			c.ioCommand <- ioCheckIdle
			<-c.ioIdle
			//delete all close(c.events) in order to prevent channel closed when server still running
		}
	}
}
