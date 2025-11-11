package stubs

// for Broker

var BrokerStart = "Broker.Start"
var BrokerCount = "Broker.AliveCellsCount"
var BrokerSave = "Broker.SaveCurrent"
var BrokerPause = "Broker.Pause"
var BrokerShutdown = "Broker.Shutdown"

type BrokerRequest struct {
	World       [][]uint8
	Turns       int
	ImageHeight int
	ImageWidth  int
	Workers     []string
}

type BrokerResponse struct {
	World      [][]uint8
	Turn       int
	AliveCells int
	Paused     bool
	Done       bool
	Err        string
}

//for workers

var WorkerStep = "Worker.Step"

type WorkerRequest struct {
	World  [][]uint8
	StartY int
	EndY   int
	Width  int
	Height int
}

type WorkerResponse struct {
	RowRes [][]uint8
}
