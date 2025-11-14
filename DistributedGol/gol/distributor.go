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
	server := "23.20.202.80:8030" // ip of AWS instance of broker!
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

	//RPC AliveCellsCount
	ticker := time.NewTicker(2 * time.Second)
	done := make(chan bool)

	//added : State tracking to ensure AliveCellsCount reports strictly increasing turn numbers
	lastAliveTurnSent := -1
	startAliveTicker := func() {
		// Restart AliveCellsCount goroutine after pause
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
					// old code was always sending data
					// c.events <- AliveCellsCount{CompletedTurns: aliveRes.Turn, CellsCount: aliveRes.AliveCells}

					//added: Skip events with non-increasing CompletedTurns
					if aliveRes.Turn <= lastAliveTurnSent {
						// skip stale or duplicate turn reports
						continue
					}
					lastAliveTurnSent = aliveRes.Turn
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
	}
	startAliveTicker()

	//added: Variables for tracking state and handling finish logic
	pausedLocal := false // Client-side pause flag, not relying on the server's response
	finalized := false   // Avoid duplicate finalization
	type pend struct {
		has   bool
		world [][]uint8
	}
	//pendingFinalize := pend{} // Holds data if rpcDone arrives during pause, processed on resume
	lastPausedTurn := 0 // Used to send Executing event when resuming

	finalize := func(finalWorld [][]uint8, finalTurn int) {
		if finalized {
			return
		}
		//receive result and updates world & turn
		world = finalWorld
		//stop the time ticker
		done <- true

		saveCurWorld(p, c, world, finalTurn)

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

		c.events <- FinalTurnComplete{CompletedTurns: finalTurn, Alive: aliveCells}

		// Make sure that the Io has finished any output before exiting.
		c.ioCommand <- ioCheckIdle
		<-c.ioIdle

		c.events <- StateChange{turn, Quitting}

		// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
		close(c.events)
		finalized = true
	}

	//added:Run ExecuteGol asynchronously so the key input loop can start imediately
	// old code : blocking call
	// err = client.Call(stubs.EngineStart, req, res)
	// if err != nil {
	// 	log.Fatal("fail to use ExecuteGol: ", err)
	// }
	rpcDone := make(chan error, 1)
	go func() {
		rpcDone <- client.Call(stubs.EngineStart, req, res)
	}()

	for { // start for loop
		select {
		//added: Monitor for simulation completion or error
		case callErr := <-rpcDone:
			if callErr != nil {
				log.Fatal("fail to use ExecuteGol: ", callErr)
			}
			// If paused, delay finalization. otherwise, finalize immediately
			if pausedLocal {
				// Keep result pending to finalize right after resume
				//pendingFinalize = pend{has: true, world: res.NewWorld}
			} else {
				finalize(res.NewWorld, p.Turns)
				return
			}

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
				var saveRes stubs.Response
				if err := client.Call(stubs.EngineSave, stubs.Request{}, &saveRes); err == nil {
					// save current world
					saveCurWorld(p, c, saveRes.NewWorld, saveRes.Turn)
				}

				// stop Alive ticker
				done <- true

				// wiat for IO work finish
				c.ioCommand <- ioCheckIdle
				<-c.ioIdle

				// q event should use the turn that was actually saved.
				quitTurn := saveRes.Turn
				c.events <- StateChange{quitTurn, Quitting}

				close(c.events)
				return

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
				return

			case 'p':
				var pauseRes stubs.Response
				if err := client.Call(stubs.EnginePaused, stubs.Request{}, &pauseRes); err != nil {
					log.Fatal("fail to toggle pause: ", err)
				}

				// toggle the local flag
				newPaused := !pausedLocal
				pausedLocal = newPaused

				if newPaused {
					lastPausedTurn = pauseRes.Turn
					fmt.Printf("Turn %d is being processed\n", lastPausedTurn)
					c.events <- StateChange{lastPausedTurn, Paused}
					// Keep the ticker running (you can simply ignore Alive events if needed)
				} else {
					fmt.Println("Continuing")
					c.events <- StateChange{lastPausedTurn, Executing}
				}

			default:
				time.Sleep(100 * time.Millisecond)
			}

		}
	} // end for loop

}
