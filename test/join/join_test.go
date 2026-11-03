package join_test

import (
	"context"
	"fmt"
	"testing"

	"dinodb/test/utils"

	"dinodb/pkg/database"
	"dinodb/pkg/entry"
	"dinodb/pkg/hash"
	"dinodb/pkg/join"
)

func TestJoin(t *testing.T) {
	t.Run("Empty", TestEmptyJoin)
	t.Run("Simple", TestSimpleJoin)
	t.Run("KeyOnValue", TestJoinKeyOnValue)
	t.Run("ValueOnValue", TestJoinValueOnValue)
	t.Run("KeysOneToOne", TestJoinKeysOneToOne)
	t.Run("ValuesManyToMany", TestJoinValuesManyToMany)
}

// Mod vals by this value to prevent hardcoding tests
var join_salt = utils.Salt

func setupJoin(t *testing.T) (*hash.HashIndex, *hash.HashIndex) {
	t.Parallel() // enable tests to run in parallel
	// Init the first database
	dbName1 := utils.GetTempDbFile(t)
	index1, err := hash.OpenTable(dbName1)
	if err != nil {
		t.Error(err)
	}

	// Init the second database
	dbName2 := utils.GetTempDbFile(t)
	index2, err := hash.OpenTable(dbName2)
	if err != nil {
		t.Error(err)
	}

	utils.EnsureCleanup(t, func() {
		// Don't check close error since we are only concerned with resource cleanup
		_ = index1.Close()
		_ = index2.Close()
	})
	return index1, index2
}

func insertIntoIndex(t *testing.T, index database.Index, key int64, value int64) {
	err := index.Insert(key, value)
	if err != nil {
		t.Fatalf("Failed to insert key-value pair {%d, %d}: %q", key, value, err)
	}
}

func getResults(t *testing.T, index1 database.Index, index2 database.Index, joinOnLeftKey bool, joinOnRightKey bool) []join.EntryPair {
	// Create context.
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	// Join the indices; set up cleanup.
	resultsChan, _, group, cleanupCallback, err := join.Join(ctx, index1, index2, joinOnLeftKey, joinOnRightKey)
	if cleanupCallback != nil {
		defer cleanupCallback()
	}
	if err != nil {
		t.Fatal("Failed initial setup steps of join:", err)
	}

	// Iterate through results.
	done := make(chan bool)
	results := make([]join.EntryPair, 0)
	go func() {
		for {
			pair, valid := <-resultsChan
			if !valid {
				break
			}
			results = append(results, pair)
		}
		done <- true
	}()

	// Wait for the join to finish (either b/c of an error or completing)
	err = group.Wait()
	close(resultsChan)
	<-done

	if err != nil {
		t.Fatal("Failed while executing join:", err)
	}
	return results
}

/*HELPER FUNCTIONS TO CHECK VALUES OF MATCHES*/

// Generates a unique string key for an EntryPair based on its contents
func entryPairKey(e join.EntryPair) string {
	return fmt.Sprintf("%d:%d-%d:%d", e.L.Key, e.L.Value, e.R.Key, e.R.Value)
}

// Checks that 2 maps with custom entryPairKeys are equivalent
func equalEntryPairs(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		_, found := b[k]
		if !found {
			return false
		}
	}
	return true
}

/* END OF HELPER FUNCTIONS */

/* TEST FUNCTIONS BELOW */

// Checks that a join with no matching keys produces no resulting pairs
func TestEmptyJoin(t *testing.T) {
	index1, index2 := setupJoin(t)

	// Insert entries.
	for i := int64(0); i < 10; i++ {
		insertIntoIndex(t, index1, i, i%join_salt)
		insertIntoIndex(t, index2, i+100, i%join_salt)
	}

	// Get and check results.
	results := getResults(t, index1, index2, true, true)
	if len(results) != 0 {
		t.Errorf("basic join not working; expected %v results, got %d\n", 0, len(results))
	}
}

// Tests general join functionality of joining on keys with
// no duplicate matches
func TestSimpleJoin(t *testing.T) {
	index1, index2 := setupJoin(t)

	// Insert entries.
	for i := int64(0); i < 10; i++ {
		insertIntoIndex(t, index1, i, i%join_salt)
	}
	insertIntoIndex(t, index2, 5, 5%join_salt)
	insertIntoIndex(t, index2, 6, 6%join_salt)

	// Get and check the results.
	results := getResults(t, index1, index2, true, true)

	// Check for expected size of results
	expectedNumResults := 2
	if len(results) != expectedNumResults {
		t.Errorf("wrong number of join results; expected %v results, got %d\n", 2, len(results))
	}

	// Check for expected values of results
	// Use a map as a set to check the existence of all values
	expectedResultsMap := make(map[string]struct{})
	for i := int64(5); i <= 6; i++ {
		lResult := entry.New(i, i%join_salt)
		rResult := entry.New(i, i%join_salt)
		entryPair := join.EntryPair{L: lResult, R: rResult}
		key := entryPairKey(entryPair)
		expectedResultsMap[key] = struct{}{}
	}

	resultsMap := make(map[string]struct{})
	for _, result := range results {
		key := entryPairKey(result)
		resultsMap[key] = struct{}{}
	}

	equal := equalEntryPairs(expectedResultsMap, resultsMap)
	if !equal {
		t.Errorf("join results have incorrect values")
	}
}

// join on left key and right value
func TestJoinKeyOnValue(t *testing.T) {
	index1, index2 := setupJoin(t)

	// insert right index entries
	insertIntoIndex(t, index2, 5, 5%join_salt)
	insertIntoIndex(t, index2, 10%join_salt, 6)

	// Check for expected values of results
	// Use a map as a set to check the existence of all values
	expectedResultsMap := make(map[string]struct{})
	// Insert left index entries and store expected results
	for i := int64(0); i < 10; i++ {
		insertIntoIndex(t, index1, i, i%join_salt)
		if i == (5 % join_salt) {
			lResult := entry.New(i, i%join_salt)
			rResult := entry.New(5, 5%join_salt)
			entryPair := join.EntryPair{L: lResult, R: rResult}
			key := entryPairKey(entryPair)
			expectedResultsMap[key] = struct{}{}
		}
		if i == 6 {
			lResult := entry.New(i, i%join_salt)
			rResult := entry.New(10%join_salt, 6)
			entryPair := join.EntryPair{L: lResult, R: rResult}
			key := entryPairKey(entryPair)
			expectedResultsMap[key] = struct{}{}
		}
	}

	// Get and check results.
	results := getResults(t, index1, index2, true, false)
	if len(results) != 2 {
		t.Errorf("basic join not working; expected %v results, got %d\n", 2, len(results))
	}

	resultsMap := make(map[string]struct{})
	for _, result := range results {
		key := entryPairKey(result)
		resultsMap[key] = struct{}{}
	}

	equal := equalEntryPairs(expectedResultsMap, resultsMap)
	if !equal {
		t.Errorf("join results have incorrect values")
	}
}

// join on left and right index values
func TestJoinValueOnValue(t *testing.T) {
	index1, index2 := setupJoin(t)

	// insert into right index
	insertIntoIndex(t, index2, 5, 5%join_salt)
	insertIntoIndex(t, index2, 10%join_salt, 6)

	// Check for expected values of results
	// Use a map as a set to check the existence of all values
	expectedResultsMap := make(map[string]struct{})
	// Insert entries into left index.
	for i := int64(0); i < 10; i++ {
		curValue := i % join_salt
		insertIntoIndex(t, index1, i, curValue)
		if curValue == 5%join_salt {
			lResult := entry.New(i, i%join_salt)
			rResult := entry.New(5, 5%join_salt)
			entryPair := join.EntryPair{L: lResult, R: rResult}
			key := entryPairKey(entryPair)
			expectedResultsMap[key] = struct{}{}
		}
		if curValue == 6 {
			lResult := entry.New(i, i%join_salt)
			rResult := entry.New(10%join_salt, 6)
			entryPair := join.EntryPair{L: lResult, R: rResult}
			key := entryPairKey(entryPair)
			expectedResultsMap[key] = struct{}{}
		}
	}

	// Get and check results length
	results := getResults(t, index1, index2, false, false)
	if len(results) != 2 {
		t.Errorf("basic join not working; expected %v results, got %d\n", 2, len(results))
	}

	// check results values
	resultsMap := make(map[string]struct{})
	for _, result := range results {
		key := entryPairKey(result)
		resultsMap[key] = struct{}{}
	}

	equal := equalEntryPairs(expectedResultsMap, resultsMap)
	if !equal {
		t.Errorf("join results have incorrect values")
	}
}

// a join where all the keys do not have multiple matches
func TestJoinKeysOneToOne(t *testing.T) {
	tests := map[string]int64{
		"Ten":      10,
		"Hundred":  100,
		"Thousand": 1_000,
	}

	for name, numInserts := range tests {
		t.Run(name, func(t *testing.T) {
			index1, index2 := setupJoin(t)

			// Insert entries.
			for i := int64(0); i < numInserts; i++ {
				insertIntoIndex(t, index1, i, i%join_salt)
				insertIntoIndex(t, index2, i, i%join_salt)
			}

			// Get and check results.
			results := getResults(t, index1, index2, true, true)
			if int64(len(results)) != numInserts {
				t.Errorf("basic join not working; expected %v results, got %d\n", numInserts, len(results))
			}
		})
	}
}

// stress test joining on value
// im not checking the values of pairs produced for this...
func TestJoinValuesManyToMany(t *testing.T) {
	index1, index2 := setupJoin(t)

	// Insert entries.
	for i := int64(0); i < 128; i++ {
		insertIntoIndex(t, index1, i, 0)
		insertIntoIndex(t, index2, i, 0)
	}

	// Get and check results.
	results := getResults(t, index1, index2, false, false)
	if len(results) != 16384 {
		t.Errorf("basic join not working; expected %v results, got %d\n", 16384, len(results))
	}
}
