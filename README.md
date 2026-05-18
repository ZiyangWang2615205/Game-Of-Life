# Game of Life — Parallel and Distributed Implementation

This project implements Conway's Game of Life in Go, with both a parallel single-machine version and a distributed version designed to run across AWS nodes.

The Game of Life is a cellular automaton where each cell on a 2D grid is either alive or dead. At each turn, the next state of every cell is determined by the states of its eight neighbouring cells. This project focuses on implementing the simulation efficiently using Go concurrency, worker goroutines, RPC communication, and distributed computation.

> This repository is a public version of a university team project. Sensitive configuration files, private deployment details, and unnecessary coursework-specific files may have been removed.

---
# Introduction
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
