package join

import (
	"context"
	"fmt"
	"io"
	"strings"

	"dinodb/pkg/database"
	"dinodb/pkg/repl"
)

func JoinRepl(db *database.Database) *repl.REPL {
	r := repl.NewRepl()
	r.AddCommand("join", func(payload string, replConfig *repl.REPLConfig) (string, error) {
		return HandleJoin(db, payload)
	}, "Joins two tables. usage: join <table1> <key/val for table1> on <table2> <key/val for table2>")
	return r
}

// HandleJoin is the REPL command for joining 2 tables on keys/values
func HandleJoin(db *database.Database, payload string) (output string, err error) {
	fields := strings.Fields(payload)
	numFields := len(fields)
	// Usage: join <table1> <key/val for table1> on <table2> <key/val for table2>
	if numFields != 6 || fields[3] != "on" || (fields[2] != "key" && fields[2] != "val") || (fields[5] != "key" && fields[5] != "val") {
		return "", fmt.Errorf("usage: join <table1> <key/val for table1> on <table2> <key/val for table2>")
	}
	table1Name := fields[1]
	table1, err := db.GetTable(table1Name)
	if err != nil {
		return "", fmt.Errorf("find error: %v", err)
	}
	table2Name := fields[4]
	table2, err := db.GetTable(table2Name)
	if err != nil {
		return "", fmt.Errorf("find error: %v", err)
	}
	joinOnLeftKey := fields[2] == "key"
	joinOnRightKey := fields[5] == "key"
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()
	resultsChan, _, group, cleanupCallback, err := Join(ctx, table1, table2, joinOnLeftKey, joinOnRightKey)
	if cleanupCallback != nil {
		defer cleanupCallback()
	}
	if err != nil {
		return "", err
	}
	w := new(strings.Builder)
	done := make(chan bool)
	go func() {
		for {
			pair, valid := <-resultsChan
			if !valid {
				break
			}
			io.WriteString(w, fmt.Sprintf("{(%v, %v), (%v, %v)}\n",
				pair.L.Key, pair.L.Value, pair.R.Key, pair.R.Value))
		}
		done <- true
	}()
	err = group.Wait()
	close(resultsChan)
	<-done
	if err != nil {
		return "", fmt.Errorf("join error: %v", err)
	}
	return w.String(), nil
}
