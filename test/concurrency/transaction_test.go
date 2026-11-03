package concurrency_test

import (
	"dinodb/pkg/concurrency"
	"dinodb/pkg/database"
	"testing"
	"time"

	"github.com/google/uuid"
)

type LockCommand struct {
	done bool
	key  int64
	lock bool
	lt   concurrency.LockType
}

func setupTransaction(t *testing.T) (*concurrency.TransactionManager, database.Index) {
	// TODO: test transaction manager with hash indices too
	index := setupIndex(t, database.BTreeIndexType)

	lm := concurrency.NewResourceLockManager()
	tm := concurrency.NewTransactionManager(lm)
	return tm, index
}

func getTransactionThread() (uuid.UUID, chan LockCommand) {
	tid := uuid.New()
	ch := make(chan LockCommand, BUFFER_SIZE)
	return tid, ch
}

func handleTransactionThread(tm *concurrency.TransactionManager, table database.Index, tid uuid.UUID, ch chan LockCommand, errch chan error) {
	var ld LockCommand
	var err error
	tm.Begin(tid)
	for {
		// Get next command
		ld = <-ch
		// Terminate if done
		if ld.done {
			break
		}
		// Lock or unlock
		if ld.lock {
			err = tm.Lock(tid, table, ld.key, ld.lt)
		} else {
			err = tm.Unlock(tid, table, ld.key, ld.lt)
		}
		// Terminate if error
		if err != nil {
			errch <- err
			break
		}
	}
	tm.Commit(tid)
}

func sendWithDelay(ch chan LockCommand, ld LockCommand) {
	time.Sleep(DELAY_TIME)
	ch <- ld
}

func checkNoErrors(t *testing.T, errch chan error) {
	time.Sleep(10 * DELAY_TIME)
	select {
	case err, ok := <-errch:
		if ok {
			t.Error(err)
		}
	default:
		t.Log("no errors")
	}
}

func checkWasErrors(t *testing.T, errch chan error) {
	time.Sleep(10 * DELAY_TIME)
	select {
	case err, ok := <-errch:
		if ok {
			t.Log(err)
		}
	default:
		t.Error("expected an error")
	}
}

func TestTransaction(t *testing.T) {
	t.Run("Basic", testTransactionBasic)
	t.Run("Deadlock", testTransactionDeadlock)
}

func testTransactionBasic(t *testing.T) {
	tm, index := setupTransaction(t)
	errch := make(chan error, BUFFER_SIZE)
	// Set up transactions
	tid1, ch1 := getTransactionThread()
	go handleTransactionThread(tm, index, tid1, ch1, errch)
	// Sending instructions
	sendWithDelay(ch1, LockCommand{key: 0, lock: true, lt: concurrency.W_LOCK})
	sendWithDelay(ch1, LockCommand{done: true})
	// Check for errors
	checkNoErrors(t, errch)
}

func testTransactionDeadlock(t *testing.T) {
	tm, index := setupTransaction(t)
	errch := make(chan error, BUFFER_SIZE)
	// Set up transactions
	tid1, ch1 := getTransactionThread()
	go handleTransactionThread(tm, index, tid1, ch1, errch)
	tid2, ch2 := getTransactionThread()
	go handleTransactionThread(tm, index, tid2, ch2, errch)
	// Sending instructions
	sendWithDelay(ch1, LockCommand{key: 0, lock: true, lt: concurrency.W_LOCK})
	sendWithDelay(ch2, LockCommand{key: 1, lock: true, lt: concurrency.W_LOCK})
	sendWithDelay(ch1, LockCommand{key: 1, lock: true, lt: concurrency.W_LOCK})
	sendWithDelay(ch2, LockCommand{key: 0, lock: true, lt: concurrency.W_LOCK})
	sendWithDelay(ch1, LockCommand{done: true})
	sendWithDelay(ch2, LockCommand{done: true})
	// Check for errors
	checkWasErrors(t, errch)
}
