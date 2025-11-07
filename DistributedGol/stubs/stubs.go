package stubs

var EngineStart = "Engine.ExecuteGol"
var EngineCount = "Engine.AliveCellsCount"
var EngineSave = "Engine.SaveCurrent"
var EngineOver = "Engine.ShutDown"
var EnginePaused = "Engine.Paused"
var EngineResumed = "Engine.Resumed"
var EngineInit = "Engine.Initialise"

type Request struct {
	World       [][]uint8
	Turn        int
	ImageHeight int
	ImageWidth  int
}

type Response struct {
	NewWorld   [][]uint8
	AliveCells int
	Turn       int
}
