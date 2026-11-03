package concurrency

import (
	"errors"
	"sync"
)

// WaitsForGraph is a precedence graph used to keep track of whether
// there are deadlocks in transactions
type WaitsForGraph struct {
	edges []Edge       // A slice of all the Edges that we have in our graph
	mtx   sync.RWMutex // Mutex for synchronizing access to the edges slice.
}

// An Edge between transactions in a ("waits-for") Graph
// if Txn1 is waiting for a resource held by Txn2,
// then there is an Edge from Txn1 to Txn2
type Edge struct {
	from *Transaction
	to   *Transaction
}

func NewGraph() *WaitsForGraph {
	return &WaitsForGraph{edges: make([]Edge, 0)}
}

// Add an edge from `from` to `to`. Logically, `from` waits for `to`.
func (g *WaitsForGraph) AddEdge(from *Transaction, to *Transaction) {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	g.edges = append(g.edges, Edge{from: from, to: to})
}

// Remove an edge. Only removes one of these edges if multiple copies exist.
func (g *WaitsForGraph) RemoveEdge(from *Transaction, to *Transaction) error {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	toRemove := Edge{from: from, to: to}
	for i, e := range g.edges {
		if e == toRemove {
			g.edges = removeHelper(g.edges, i)
			return nil
		}
	}
	return errors.New("edge not found")
}

// Remove the element at index `i` from `list`.
func removeHelper(list []Edge, i int) []Edge {
	list[i] = list[len(list)-1]
	return list[:len(list)-1]
}

// Return true if a cycle exists; false otherwise.
func (g *WaitsForGraph) DetectCycle() (hasCycle bool) {
	panic("Not yet implemented!")
}

// depth-first search function to help detect cycles in a graph.
// Returns true if a cycle is reachable from `from`.
//
// `seen` tracks the state of each transaction:
//   - not in the map: not visited yet
//   - true:  on the current search path
//   - false: fully explored, and no cycle is reachable from it
//
// Pass in an empty map (or reuse one map across calls to skip re-exploring transactions).
func dfs(g *WaitsForGraph, from *Transaction, seen map[*Transaction]bool) bool {
	seen[from] = true // `from` is now on the current path
	// Go through each edge out of `from`.
	for _, e := range g.edges {
		if e.from != from {
			continue
		}
		onPath, visited := seen[e.to]
		// An edge back to a transaction on the current path is a cycle.
		if visited && onPath {
			return true
		}
		// Explore transactions we haven't visited yet.
		if !visited && dfs(g, e.to, seen) {
			return true
		}
	}
	seen[from] = false // done with `from`; no cycle through it
	return false
}
