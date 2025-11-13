package stubs

var EngineStart = "Engine.ExecuteGol"
var EngineCount = "Engine.AliveCellsCount"

//add new RPC for keyPress problems

var EnginePause = "Engine.Pause"
var EngineResume = "Engine.Resume"
var EngineKill = "Engine.Kill"
var EngineGetWorld = "Engine.GetWorld"

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
	OK         bool // added for control Pause/Resume/Kill
}

var WorkerCompute = "Worker.Compute"

type SliceRequest struct {
	Slice      [][]uint8
	AboveRow   []uint8
	BelowRow   []uint8
	ImageWidth int
}

type SliceResponse struct {
	NewSlice [][]uint8
}
