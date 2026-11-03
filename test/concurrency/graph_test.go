package concurrency_test

import (
	"dinodb/pkg/concurrency"
	"testing"
)

func TestDeadlock(t *testing.T) {
	t.Run("Simple", testDeadlockSimple)
}

func testDeadlockSimple(t *testing.T) {
	t1 := concurrency.Transaction{}
	t2 := concurrency.Transaction{}
	g := concurrency.NewGraph()
	g.AddEdge(&t1, &t2)
	g.AddEdge(&t2, &t1)
	if !g.DetectCycle() {
		t.Error("failed to detect cycle")
	}
}
