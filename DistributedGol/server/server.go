package main

import (
	"flag"
	"log"
	"net"
	"net/rpc"

	"uk.ac.bris.cs/gameoflife/stubs"
)

//This file provides the Worker RPC service called by the broker, replacing the original single-node Engine server.
//The controller (distributor.go) still treats only the broker as the Engine and never communicates directly with individual workers.

type Worker struct{} // [NEW]

// countNeighbours calculates the number of live neighbors (8-neighborhood) for a given cell, using both the stripe and its top/bottom halo rows.
func countNeighbours(x, y int, stripe [][]uint8, width int, haloTop, haloBottom []uint8) int { // [NEW]
	h := len(stripe)
	count := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			nx := (x + dx + width) % width
			ny := y + dy

			var v uint8
			switch {
			case ny < 0:
				v = haloTop[nx]
			case ny >= h:
				v = haloBottom[nx]
			default:
				v = stripe[ny][nx]
			}
			if v == 255 {
				count++
			}
		}
	}
	return count
}

// Step advances one turn of Game of Life for a single stripe.
func (w *Worker) Step(req stubs.StripeRequest, res *stubs.StripeResponse) error {
	stripe := req.Stripe
	h := req.LocalH
	wth := req.ImageWidth

	newStripe := make([][]uint8, h)
	for y := 0; y < h; y++ {
		newStripe[y] = make([]uint8, wth)
	}

	alive := 0
	for y := 0; y < h; y++ {
		for x := 0; x < wth; x++ {
			neigh := countNeighbours(x, y, stripe, wth, req.HaloTop, req.HaloBottom)
			cur := stripe[y][x]
			if cur == 255 {
				if neigh == 2 || neigh == 3 {
					newStripe[y][x] = 255
					alive++
				} else {
					newStripe[y][x] = 0
				}
			} else {
				if neigh == 3 {
					newStripe[y][x] = 255
					alive++
				} else {
					newStripe[y][x] = 0
				}
			}
		}
	}

	res.NewStripe = newStripe
	res.AliveCount = alive
	return nil
}

// Ping/Shutdown can be used by the broker to check or terminate a worker.
func (w *Worker) Ping(_ struct{}, _ *struct{}) error {
	return nil
}

func (w *Worker) Shutdown(_ struct{}, _ *struct{}) error {
	return nil
}

func main() {
	pAddr := flag.String("port", "8031", "Port to listen on")
	flag.Parse()

	if err := rpc.RegisterName("Worker", &Worker{}); err != nil {
		log.Fatal("failed to register Worker:", err)
	}

	ln, err := net.Listen("tcp", ":"+*pAddr)
	if err != nil {
		log.Fatal("worker listen error:", err)
	}
	defer ln.Close()

	log.Printf("Worker listening on %s\n", *pAddr)
	rpc.Accept(ln)
}

//  go run server.go -port 8031
