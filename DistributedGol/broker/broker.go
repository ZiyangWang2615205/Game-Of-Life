package broker

import (
	"flag"
	"log"
	"net"
	"net/rpc"

	"uk.ac.bris.cs/gameoflife/stubs"
)

type Broker struct {
	client *rpc.Client
}

func (b *Broker) forward(tag, method string, req stubs.Request, res *stubs.Response) error {
	log.Printf("[Broker] %s: calling %s\n", tag, method)
	err := b.client.Call(method, req, res)
	if err != nil {
		log.Printf("[Broker] %s: %s error: %v\n", tag, method, err)
	} else {
		log.Printf("[Broker] %s: %s finished (Turn=%d Alive=%d OK=%v)\n",
			tag, method, res.Turn, res.AliveCells, res.IsPaused)
	}
	return err
}

func (b *Broker) ExecuteGol(req stubs.Request, res *stubs.Response) error {
	return b.forward("ExecuteGol", stubs.EngineStart, req, res)
}

func (b *Broker) AliveCellsCount(req stubs.Request, res *stubs.Response) error {
	return b.forward("AliveCellsCount", stubs.EngineCount, stubs.Request{}, res)
}

func (b *Broker) Pause(req stubs.Request, res *stubs.Response) error {
	return b.forward("Save", stubs.EngineSave, stubs.Request{}, res)
}

func (b *Broker) Resume(req stubs.Request, res *stubs.Response) error {
	return b.forward("Shutdown", stubs.EngineOver, stubs.Request{}, res)
}

func (b *Broker) Kill(req stubs.Request, res *stubs.Response) error {
	return b.forward("Pause", stubs.EnginePaused, stubs.Request{}, res)
}

func main() {
	worker := flag.String("worker", "54.235.5.92:8030", "Worker engine address")

	port := flag.String("port", "8030", "Broker listen port")
	flag.Parse()

	client, err := rpc.Dial("tcp", *worker)
	if err != nil {
		log.Fatal("Failed to connect to worker:", err)
	}

	b := &Broker{client: client}

	if err := rpc.RegisterName("Engine", b); err != nil {
		log.Fatal("Failed to register broker as Engine:", err)
	}

	ln, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		log.Fatal("Broker listen error:", err)
	}

	log.Printf("Broker started on %s, forwarding to worker %s\n",
		*port, *worker,
	)

	rpc.Accept(ln)
}
