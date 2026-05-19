# Game of Life — Parallel and Distributed Implementation

This project implements Conway's Game of Life in Go, with both a parallel single-machine version and a distributed version designed to run across AWS nodes.

The Game of Life is a cellular automaton where each cell on a 2D grid is either alive or dead. At each turn, the next state of every cell is determined by the states of its eight neighbouring cells. This project focuses on implementing the simulation efficiently using Go concurrency, worker goroutines, RPC communication, and distributed computation.

> This repository is a public version of a university team project. Sensitive configuration files, private deployment details, and unnecessary coursework-specific files may have been removed.

---

## Introduction
The British mathematician John Horton Conway devised a cellular automaton named `The Game of Life`.

The game resides on a 2-valued 2D matrix, i.e. a binary image, where the cells can either be `alive` (pixel value 255 - white) or `dead` (pixel value 0 - black).

The game evolution is determined by its initial state and requires no further input.

Every cell interacts with its eight neighbour pixels: cells that are horizontally, vertically, or diagonally adjacent.

At each matrix update in time the following transitions may occur to create the next evolution of the domain:

any live cell with fewer than two live neighbours dies
any live cell with two or three live neighbours is unaffected
any live cell with more than three live neighbours dies
any dead cell with exactly three live neighbours becomes alive
Consider the image to be on a closed domain (pixels on the top row are connected to pixels at the bottom row, pixels on the right are connected to pixels on the left and vice versa).

A user can only interact with the Game of Life by creating an initial configuration and observing how it evolves.

Note that evolving such complex, deterministic systems is an important application of scientific computing, often making use of parallel architectures and concurrent programs running on large computing farms.

Our task is to design and implement programs which simulate the Game of Life on an image matrix.

---

## Project Overview

The project contains two main implementations:

### 1. Parallel Game of Life

The parallel implementation runs on a single machine and divides the board into sections processed by multiple worker goroutines.

Key features include:

- Single-threaded baseline implementation.
- Multi-threaded board evolution using Go goroutines.
- Worker-based parallel computation.
- Correct handling of toroidal board edges.
- Periodic reporting of alive cell counts.
- PGM image input and output.
- Keyboard controls for saving, pausing, quitting, and terminating.
- SDL-based visualisation support.
- Race-condition and deadlock-aware design.

### 2. Distributed Game of Life

The distributed implementation separates the system into multiple components that communicate over the network.

Key features include:

- Local controller responsible for I/O and user interaction.
- Remote GoL engine running as an RPC server.
- AWS-based distributed execution.
- RPC communication between controller, broker, and worker nodes.
- Distributed board partitioning.
- Collection and merging of worker results.
- Support for alive-cell reporting and PGM output.
- Clean shutdown behaviour for distributed components.

---

## Architecture

The system is structured around three main layers:

```text
Local Controller
    |
    | RPC
    v
Broker / Game Engine
    |
    | RPC
    v
Worker Nodes
```

### Local Controller

The controller handles:

- Reading input PGM images.
- Sending simulation requests to the engine.
- Receiving simulation results.
- Handling keyboard input.
- Writing output PGM images.
- Reporting simulation progress.

### Broker / Game Engine

The broker or engine handles:

- Coordinating the simulation.
- Dividing the board into chunks.
- Assigning work to worker nodes.
- Managing communication between controller and workers.
- Combining worker results into the next board state.

### Worker Nodes

Worker nodes handle:

- Receiving a slice of the board.
- Computing the next state for assigned rows.
- Returning updated board sections.
- Supporting scalable distributed computation.

---

## Technologies Used

- **Go** — main implementation language.
- **Goroutines** — parallel execution.
- **Channels** — communication between concurrent components.
- **net/rpc** — distributed communication between controller, broker, and workers.
- **AWS EC2** — deployment target for distributed worker nodes.
- **SDL** — visualisation and keyboard interaction.
- **PGM image format** — input and output board representation.
- **GitHub Actions** — automated testing / workflow support.
- **Python** — benchmark plotting and analysis scripts.

---

## Repository Structure

```text
.
├── ConcurrentGol/        # Parallel single-machine Game of Life implementation
├── DistributedGol/       # Distributed RPC/AWS Game of Life implementation
├── README.md
└── .gitignore
```

Depending on the version of the repository, some private deployment files, generated outputs, or coursework-only materials may have been removed from this public version.

---

## My Contributions

This was a team project. My main contributions focused on infrastructure, distributed system setup, workflow automation, testing, and performance analysis.

- Built and configured the AWS server environment used for the distributed implementation.
- Set up the server-client structure for the distributed Game of Life system.
- Configured RPC-based communication between local controller and remote computation components.
- Developed benchmark and plotting scripts for analysing performance across different configurations.
- Contributed to the design and implementation of the distributed computation workflow.
- Created and structured instruction/documentation pages to explain how to run and use the project.

---

## Running the Project

### Prerequisites

You need:

- Go installed.
- SDL2 installed if running the visualiser.
- Access to AWS instances if running the distributed version.

For macOS, SDL2 can be installed using:

```bash
brew install sdl2
```

For Ubuntu:

```bash
sudo apt install libsdl2-dev
```

---

## Running the Parallel Version

Navigate to the parallel implementation directory:

```bash
cd ConcurrentGol
```

Run the program:

```bash
go run .
```

Run without SDL visualisation:

```bash
go run . -headless -t 4
```

Here, `-t` specifies the number of worker threads.

---

## Running Tests

Run all tests:

```bash
go test ./tests -v
```

Run Game of Life correctness tests:

```bash
go test ./tests -v -run TestGol
```

Run alive-cell reporting tests:

```bash
go test ./tests -v -run TestAlive
```

Run PGM output tests:

```bash
go test ./tests -v -run TestPgm
```

Run keyboard-control tests:

```bash
go test ./tests -v -run TestKeyboard
```

Run SDL-related tests:

```bash
go test ./tests -v -run TestSdl
```

Run tests with the race detector:

```bash
go test ./tests -v -race
```

---

## Running the Distributed Version

Navigate to the distributed implementation directory:

```bash
cd DistributedGol
```

Start the remote GoL engine or worker process on the AWS node.

Then run the local controller from your local machine.

The distributed version is designed around RPC communication, where the controller sends requests to the remote engine and receives updated board states or progress information.

Example structure:

```text
Local Machine:
  Controller

AWS Node:
  GoL Engine / Worker
```

For multi-node execution, the broker coordinates work distribution across multiple AWS worker nodes.

---

## Keyboard Controls

The project supports interactive controls:

| Key  | Behaviour                                      |
| ---- | ---------------------------------------------- |
| `s`  | Save the current board state as a PGM image    |
| `p`  | Pause or resume the simulation                 |
| `q`  | Save the current state and quit the controller |
| `k`  | Shut down distributed components cleanly       |

---

## Performance and Benchmarking

The project includes benchmarking support to compare different implementations and worker configurations.

Performance analysis focuses on:

- Scaling behaviour as the number of worker threads increases.
- Communication overhead in the distributed version.
- Correctness under concurrent execution.
- Avoiding race conditions and deadlocks.
- Comparing single-threaded, parallel, and distributed approaches.

Example benchmark command:

```bash
go test -bench=. ./...
```

Benchmark results can be plotted using the included Python plotting scripts where available.

---

## Public Repository Notice

This repository is intended for portfolio and CV demonstration purposes.

Some files may have been removed or simplified, including:

- Private AWS configuration.
- Credentials or environment files.
- Generated output files.
- Coursework-only private materials.
- Sensitive deployment details.

The purpose of this public version is to demonstrate the design, implementation approach, concurrency model, distributed-system structure, and testing workflow of the project.

---

## Skills Demonstrated

This project demonstrates experience with:

- Concurrent programming in Go.
- Goroutines and channels.
- Worker-pool design.
- RPC-based distributed systems.
- AWS deployment.
- Parallel algorithm design.
- Image-based input/output processing.
- Automated testing.
- CI/CD workflow configuration.
- Performance benchmarking and analysis.

---

# Addition Information
All documentation is available [here](https://uob-csa.github.io/gol-docs/)
