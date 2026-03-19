package main

import (
	"testing"
)

func TestPersisterEnqueue(t *testing.T) {
	p := &Persister{ch: make(chan persistOp, 100)}
	p.Enqueue("INSERT INTO test VALUES($1)", 1)
	select {
	case op := <-p.ch:
		if op.query != "INSERT INTO test VALUES($1)" {
			t.Fatalf("unexpected query: %s", op.query)
		}
		if len(op.args) != 1 || op.args[0] != 1 {
			t.Fatal("unexpected args")
		}
	default:
		t.Fatal("expected op in channel")
	}
}

func TestPersisterBatchAccumulation(t *testing.T) {
	p := &Persister{ch: make(chan persistOp, 100)}
	for i := 0; i < 10; i++ {
		p.Enqueue("SELECT $1", i)
	}
	if len(p.ch) != 10 {
		t.Fatalf("expected 10 ops, got %d", len(p.ch))
	}
}
