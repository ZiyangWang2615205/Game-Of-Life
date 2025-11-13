package stubs

var EngineStart = "Engine.ExecuteGol"
var EngineCount = "Engine.AliveCellsCount"

// key interaction RPC
var EngineSave = "Engine.SaveCurrent"
var EngineOver = "Engine.ShutDown"
var EnginePaused = "Engine.Paused"

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
	IsPaused   bool
}

// ===== NEW: worker-side RPC for multi-node distributed execution =====

// Worker RPC method names used by the broker to talk to AWS worker nodes.
// These do not affect the existing controller↔engine protocol.
var WorkerStep = "Worker.Step"         // [NEW]
var WorkerPing = "Worker.Ping"         // [NEW]
var WorkerShutdown = "Worker.Shutdown" // [NEW]

// StripeRequest describes a vertical strip of rows plus its halo.
// The broker sends one StripeRequest per worker for every turn.
type StripeRequest struct { // [NEW]
	Stripe     [][]uint8 // local rows owned by this worker
	HaloTop    []uint8   // one row immediately above Stripe
	HaloBottom []uint8   // one row immediately below Stripe

	ImageWidth int // full board width
	LocalH     int // len(Stripe)
}

// StripeResponse contains the updated strip after a single turn.
type StripeResponse struct { // [NEW]
	NewStripe  [][]uint8 // next-generation rows for this worker
	AliveCount int       // optional per-stripe alive count (not required by harness)
}
