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
	server := "98.93.12.89:8030"
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

	//------------------------- 신규: AliveCellsCount 단조 증가 보장용 상태 -------------------------
	lastAliveTurnSent := -1
	startAliveTicker := func() {
		// 기존 고루틴을 재가동할 때 사용
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
					// 기존: 항상 전송
					// c.events <- AliveCellsCount{CompletedTurns: aliveRes.Turn, CellsCount: aliveRes.AliveCells}

					//------------------------- 신규: 이전에 보낸 CompletedTurns 보다 작거나 같으면 드롭 -------------------------
					if aliveRes.Turn <= lastAliveTurnSent {
						// skip stale or duplicate turn reports
						continue
					}
					lastAliveTurnSent = aliveRes.Turn
					c.events <- AliveCellsCount{
						CompletedTurns: aliveRes.Turn,
						CellsCount:     aliveRes.AliveCells,
					}
					//------------------------- 신규 끝 -------------------------

				case <-done:
					ticker.Stop()
					return
				}
			}
		}()
	}
	startAliveTicker()
	//------------------------- 신규 끝 ------------------------------------------------------------

	//------------------------- 신규: 상태/마감 관리 변수 & 마감 함수 -------------------------
	pausedLocal := false // 클라이언트가 인지한 일시정지 상태(서버 응답에 의존하지 않음)
	finalized := false   // 중복 마감 방지
	type pend struct {
		has   bool
		world [][]uint8
	}
	pendingFinalize := pend{} // Paused 중 rpcDone이 온 경우, 재개 때 처리
	lastPausedTurn := 0       // 재개 시 Executing 이벤트용으로 사용
	//------------------------- 신규 끝 ------------------------------------------------------------

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

	//------------------------- 신규: ExecuteGol을 비동기로 실행하여 키 루프가 즉시 시작되도록 함 -------------------------
	// 기존(블로킹) 호출
	// err = client.Call(stubs.EngineStart, req, res)
	// if err != nil {
	// 	log.Fatal("fail to use ExecuteGol: ", err)
	// }
	rpcDone := make(chan error, 1)
	go func() {
		rpcDone <- client.Call(stubs.EngineStart, req, res)
	}()
	//------------------------- 신규 끝 ------------------------------------------------------------

	for true { // just for loop
		select {
		//------------------------- 신규: 시뮬레이션 완료/에러 감시 -------------------------
		case callErr := <-rpcDone:
			if callErr != nil {
				log.Fatal("fail to use ExecuteGol: ", callErr)
			}
			// Paused 중이면 마감 보류, 아니면 즉시 마감
			if pausedLocal {
				// 재개되면 곧바로 마감하기 위해 보류
				pendingFinalize = pend{has: true, world: res.NewWorld}
			} else {
				finalize(res.NewWorld, p.Turns)
				return
			}
		//------------------------- 신규 끝 ------------------------------------------------

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
				//------------------------- 신규: 최신 상태 저장 후 종료 (Pause 중에도 동작) -------------------------
				var saveRes stubs.Response
				if err := client.Call(stubs.EngineSave, stubs.Request{}, &saveRes); err == nil {
					saveCurWorld(p, c, saveRes.NewWorld, saveRes.Turn)
				}
				// 기존: done <- true 만 하고 저장 없이 종료
				// done <- true
				done <- true
				// Make sure that the Io has finished any output before exiting.
				c.ioCommand <- ioCheckIdle
				<-c.ioIdle

				c.events <- StateChange{turn, Quitting}

				// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
				close(c.events)
				return
				//------------------------- 신규 끝 -------------------------

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
				//If p is pressed, pause the processing on the AWS node and have the controller print the current turn that is being processed.
				//If p is pressed again resume the processing and have the controller print Continuing.
				var pauseRes stubs.Response

				//------------------------- 신규: Pause 경쟁 줄이기 위해 Alive 틱커 잠시 중단 -------------------------
				// (EngineCount가 월드 전체를 스캔하며 뮤텍스를 오래 잡을 수 있으므로, p 처리 전에 중단)
				done <- true
				// 새 틱커 인스턴스 준비 (재시작용)
				ticker = time.NewTicker(2 * time.Second)
				//------------------------- 신규 끝 -------------------------

				//------------------------- 신규: 서버에 토글 RPC 호출 -------------------------
				if err := client.Call(stubs.EnginePaused, stubs.Request{}, &pauseRes); err != nil {
					log.Fatal("fail to toggle pause: ", err)
				}
				//------------------------- 신규 끝 -------------------------
				//------------------------- 신규: 서버의 IsPaused 의미가 뒤집혀도 클라가 자체 토글로 보장 -------------------------
				newPaused := !pausedLocal
				pausedLocal = newPaused
				if newPaused {
					// 첫 p: 반드시 Paused 이벤트를 보냄
					// fmt.Printf("Turn %d is being processed\n", pauseRes.Turn)
					// c.events <- StateChange{pauseRes.Turn, Paused}
					//------------------------- 변경: 로컬에도 턴 기록 -------------------------
					lastPausedTurn = pauseRes.Turn
					fmt.Printf("Turn %d is being processed\n", lastPausedTurn)
					c.events <- StateChange{lastPausedTurn, Paused}
					//------------------------- 변경 끝 -------------------------

					//------------------------- 신규: 일시정지 중에도 Alive 이벤트는 필요 없으므로 재시작은 하지 않아도 무방
					// (테스트에서는 키 이벤트만 검증) 그래도 일관성을 위해 재시작해 둔다.
					startAliveTicker()
					//------------------------- 신규 끝 -------------------------

				} else {
					// 재개: 테스트는 Executing 이벤트를 기대
					// 만약 시뮬이 이미 끝나서 rpcDone이 대기 중이면, Executing을 먼저 내보내고 즉시 마감
					if pendingFinalize.has && !finalized {
						// Executing 이벤트를 먼저 보내 기대를 충족
						// fmt.Println("Continuing")
						// c.events <- StateChange{pauseRes.Turn, Executing}
						//------------------------- 변경: Executing의 CompletedTurns를 마지막 Paused 기준으로 보냄 -------------------------
						fmt.Println("Continuing")
						c.events <- StateChange{lastPausedTurn, Executing}
						//------------------------- 변경 끝 -------------------------
						finalize(pendingFinalize.world, p.Turns)
						return
					}
					// fmt.Println("Continuing")
					// c.events <- StateChange{pauseRes.Turn, Executing}
					//------------------------- 변경: 일반 재개도 lastPausedTurn로 Executing 이벤트 -------------------------
					fmt.Println("Continuing")
					c.events <- StateChange{lastPausedTurn, Executing}
					//------------------------- 변경 끝 -------------------------

					//------------------------- 신규: 재개 후 Alive 틱커 재시작 -------------------------
					startAliveTicker()
					//------------------------- 신규 끝 -------------------------
				}
				//------------------------- 신규 끝 -------------------------
			default:
				time.Sleep(100 * time.Millisecond)
			}

		}
	}

	// 아래 블록은 이제 rpcDone 분기로 이동됨.
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
