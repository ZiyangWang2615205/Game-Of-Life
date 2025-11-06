package stubs

var EngineStart = "Engine.ExecuteGol"

type Request struct {
	World       [][]uint8
	Turn        int
	ImageHeight int
	ImageWidth  int
}

type Response struct {
	NewWorld [][]uint8
}
