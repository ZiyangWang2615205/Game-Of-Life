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
	server := "3.91.159.142:8050"
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
	req := stubs.BrokerRequest{
		World:       world,
		Turns:       p.Turns,
		ImageHeight: p.ImageHeight,
		ImageWidth:  p.ImageWidth,
	}

	var res stubs.BrokerResponse

	//update and send alive cells count each 2s
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
					var aliveRes stubs.BrokerResponse
					err = client.Call(stubs.BrokerCount, stubs.BrokerRequest{}, &aliveRes)
					if err != nil {
						log.Fatal("fail to use AliveCellsCount: ", err)
					}
					// send the AliveCellsCount event
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

	//pausedLocal is client-side pause flag, not relying on the server's response
	pausedLocal := false

	//finalized used for avoiding duplicate finalization
	finalized := false

	type pend struct {
		shouldPaused bool
		world        [][]uint8
	}

	pendingFinalize := pend{} // Holds data if rpcDone arrives during pause, processed on resume
	lastPausedTurn := 0       // Used to send Executing event when resuming

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

	//Run ExecuteGol asynchronously so the key input loop can start immediately
	rpcDone := make(chan error, 1)
	go func() {
		rpcDone <- client.Call(stubs.BrokerStart, req, res)
	}()

	for {
		select {
		//added: Monitor for simulation completion or error
		case dealErr := <-rpcDone:
			if dealErr != nil {
				log.Fatal("fail to use ExecuteGol: ", dealErr)
			}
			// If paused, delay finalization. otherwise, finalize immediately
			if pausedLocal {
				// Keep result pending to finalize right after resume
				pendingFinalize = pend{shouldPaused: true, world: res.World}
			} else {
				finalize(res.World, p.Turns)
				return
			}

		case key := <-keyPresses:
			switch key {
			case 's':
				//If s is pressed, the controller should generate a PGM file with the current state of the board.
				var saveRes stubs.BrokerResponse
				err := client.Call(stubs.BrokerSave, stubs.BrokerRequest{}, &saveRes)
				if err != nil {
					log.Fatal("fail to use SaveCurrent: ", err)
				}
				saveCurWorld(p, c, saveRes.World, saveRes.Turn)

			case 'q':
				//If q is pressed, close the controller client program without causing an error on the Gol server.

				//added: Save the latest state before exiting (works even while paused)
				var saveRes stubs.BrokerResponse
				if err := client.Call(stubs.BrokerSave, stubs.BrokerRequest{}, &saveRes); err == nil {
					saveCurWorld(p, c, saveRes.World, saveRes.Turn)
				}
				// Old code was only sent done <- true without saving the state
				// done <- true
				done <- true
				// Make sure that the Io has finished any output before exiting.
				c.ioCommand <- ioCheckIdle
				<-c.ioIdle

				c.events <- StateChange{turn, Quitting}

				// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
				close(c.events)
				return

			case 'k':
				//If k is pressed, all components of the distributed system are shut down cleanly, and the system outputs a PGM image of the latest state.
				var overRes stubs.BrokerResponse
				_ = client.Call(stubs.BrokerShutdown, stubs.BrokerRequest{}, &overRes)
				saveCurWorld(p, c, overRes.World, overRes.Turn)

				done <- true
				c.ioCommand <- ioCheckIdle
				<-c.ioIdle

				c.events <- StateChange{overRes.Turn, Quitting}
				close(c.events)
				return

			case 'p':
				//If p is pressed, pause the processing on the AWS node and have the controller print the current turn that is being processed.
				//If p is pressed again resume the processing and have the controller print Continuing.
				var pauseRes stubs.BrokerResponse

				//added: Temporarily stop the Alive ticker to reduce pause contention
				// (EngineCount scans the entire world and may hold the mutex for a long time, so stop before handling 'p')
				done <- true
				// Create a new ticker instance (for restarting later)
				ticker = time.NewTicker(2 * time.Second)

				//added: Send toggle RPC request to the server
				if err := client.Call(stubs.BrokerPause, stubs.BrokerRequest{}, &pauseRes); err != nil {
					log.Fatal("fail to toggle pause: ", err)
				}
				//change paused state as long as press p
				newPaused := !pausedLocal
				pausedLocal = newPaused
				if newPaused {
					//Record the turn number locally
					lastPausedTurn = pauseRes.Turn
					fmt.Printf("Turn %d is being processed\n", lastPausedTurn)
					c.events <- StateChange{lastPausedTurn, Paused}

					//added: Alive events are not needed while paused; restarting is optional
					// (The test only checks key events, but we restart for consistency)
					startAliveTicker()

				} else {
					// On resume the test expects an Executing event
					// If the simulation has already finished and rpcDone is waiting,
					// send Executing event first to satisfy the test and then finalize immediately
					if pendingFinalize.shouldPaused && !finalized {
						//changed to: Send Executing event using the last paused turn number
						fmt.Println("Continuing")
						c.events <- StateChange{lastPausedTurn, Executing}

						finalize(pendingFinalize.world, p.Turns)
						return
					}
					//changed to: When resuming normally, also send Executing event with lastPausedTurn
					fmt.Println("Continuing")
					c.events <- StateChange{lastPausedTurn, Executing}

					//added: Restart the Alive ticker after resuming
					startAliveTicker()

				}

			default:
				time.Sleep(100 * time.Millisecond)
			}

		}
	} // end for loop

	// The block below has been moved to the rpcDone case
	//receive result and updates world & turn
	// world = res.NewWorld
	// //stop the time ticker
	// done <- true
	//
	// saveCurWorld(p, c, world, p.Turns)
	//
	// // TODO: Report the final state using FinalTurnCompleteEvent.
	// aliveCells := []util.Cell{}
	// for y := 0; y < p.ImageHeight; y++ {
	// 	for x := 0; x < p.ImageWidth; x++ {
	// 		if world[y][x] == 255 {
	// 			aliveCells = append(aliveCells, util.Cell{
	// 				X: x,
	// 				Y: y,
	// 			})
	// 		}
	// 	}
	// }
	//
	// c.events <- FinalTurnComplete{CompletedTurns: p.Turns, Alive: aliveCells}
	//
	// // Make sure that the Io has finished any output before exiting.
	// c.ioCommand <- ioCheckIdle
	// <-c.ioIdle
	//
	// c.events <- StateChange{turn, Quitting}
	//
	// // Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	// close(c.events)
}
