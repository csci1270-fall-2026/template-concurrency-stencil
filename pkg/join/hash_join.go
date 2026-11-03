package join

import (
	"context"
	"os"

	"dinodb/pkg/database"
	"dinodb/pkg/entry"
	"dinodb/pkg/hash"

	"golang.org/x/sync/errgroup"
)

const JOIN_MEMORY_SIZE = 8
const NUM_SUBPARTITIONS = 4
const DEFAULT_FILTER_SIZE int64 = 1024

// Entry pair struct - output of a join.
type EntryPair struct {
	L entry.Entry
	R entry.Entry
}

// Int64 pair struct - to keep track of seen bucket pairs.
type Pair struct {
	L int64
	R int64
}

// getTempDbFile Creates and returns the name of a temporary .db file used to back a pager.
func getTempDbFile() (string, error) {
	tmpfile, err := os.CreateTemp("", "*.db")
	if err != nil {
		return "", err
	}
	// close the temp file since it is open by default and we will open it again in Pager.New()
	_ = tmpfile.Close()
	return tmpfile.Name(), nil
}

// closeTempHashTable closes the given temporary hash index and removes its backing .db and .meta files.
func closeTempHashTable(tempIndex *hash.HashIndex, dbName string) {
	tempIndex.Close()
	os.Remove(dbName)
	os.Remove(dbName + ".meta")
}

// buildHashIndex constructs a temporary hash table for all the entries in the given sourceIndex.
// The useKey argument determines whether to use each entry's original key as the key
// in the temporary hash table (if false, we use the original value as the key).
func buildHashIndex(
	sourceIndex database.Index,
	useKey bool,
) (tempIndex *hash.HashIndex, dbName string, err error) {
	dbName, err = getTempDbFile()
	if err != nil {
		return nil, "", err
	}
	// Init the temporary hash table.
	tempIndex, err = hash.OpenTable(dbName)
	if err != nil {
		return nil, "", err
	}
	// Build the hash index.
	/* SOLUTION {{{ */
	// Get a cursor to the start of the index
	cursor, err := sourceIndex.CursorAtStart()
	if err != nil {
		closeTempHashTable(tempIndex, dbName)
		return nil, "", err
	}
	// Release the cursor once we're done; a B+Tree cursor holds its leaf page pinned and locked.
	defer cursor.Close()
	// Loop through all entries.
	for {
		val, err := cursor.GetEntry()
		if err != nil {
			closeTempHashTable(tempIndex, dbName)
			return nil, "", err
		}
		// Swap keys and values if needed, this needs to be swapped back later.
		if useKey {
			tempIndex.Insert(val.Key, val.Value)
		} else {
			tempIndex.Insert(val.Value, val.Key)
		}
		if cursor.Next() {
			break
		}
	}
	return tempIndex, dbName, nil
	/* SOLUTION }}} */
}

// sendResult attempts to send a single join result to the resultsChan channel as long as the errgroup hasn't been cancelled.
func sendResult(
	ctx context.Context,
	resultsChan chan EntryPair,
	result EntryPair,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case resultsChan <- result:
		return nil
	}
}

/* SOLUTION {{{ */
// compareAndEmit probes lEntries against rEntries and sends every matching pair on resultsChan.
//
// A bloom filter is built from rEntries' keys so lEntries whose key can't possibly be present
// are skipped before the nested-loop comparison. The joinOnLeftKey and joinOnRightKey arguments
// dictate whether we originally were matching on keys or values for each side; results are
// swapped back to key/value order accordingly before being sent.
func compareAndEmit(ctx context.Context,
	resultsChan chan EntryPair,
	lEntries []entry.Entry,
	rEntries []entry.Entry,
	joinOnLeftKey bool,
	joinOnRightKey bool) error {
	// Set up the bloom filter.
	filter := CreateFilter(DEFAULT_FILTER_SIZE)
	for _, rEntry := range rEntries {
		filter.Insert(rEntry.Key)
	}
	for _, lEntry := range lEntries {
		lMatchKey := lEntry.Key
		// Check the bloom filter first.
		if !filter.Contains(lMatchKey) {
			continue
		}
		// Check all entries if the key is in the filter.
		for _, rEntry := range rEntries {
			rMatchKey := rEntry.Key
			if lMatchKey == rMatchKey {
				// Swap keys and values as needed.
				lResult := lEntry
				if !joinOnLeftKey {
					lResult = entry.New(lEntry.Value, lEntry.Key)
				}
				rResult := rEntry
				if !joinOnRightKey {
					rResult = entry.New(rEntry.Value, rEntry.Key)
				}
				err := sendResult(ctx, resultsChan, EntryPair{L: lResult, R: rResult})
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

/* SOLUTION }}} */

// RepartitionEntries splits entries into NUM_SUBPARTITIONS subpartitions by rehashing
// each entry's key with hash.MurmurHasher. Entries from a bucket pair that overflowed
// JOIN_MEMORY_SIZE get spread across smaller subpartitions so each one can be probed
// within memory.
func RepartitionEntries(entries []entry.Entry) [][]entry.Entry {
	/* SOLUTION {{{ */
	subpartitions := make([][]entry.Entry, NUM_SUBPARTITIONS)
	for _, e := range entries {
		i := hash.MurmurHasher(e.Key, NUM_SUBPARTITIONS)
		subpartitions[i] = append(subpartitions[i], e)
	}
	return subpartitions
	/* SOLUTION }}} */
}

// RepartitionAndProbe takes partitions that are larger than JOIN_MEMORY_SIZE and rehashes them to form smaller-sized
// subpartitions. This function then probes corresponding partitions, searching for matching entries.
func RepartitionAndProbe(
	ctx context.Context,
	resultsChan chan EntryPair,
	lEntries []entry.Entry,
	rEntries []entry.Entry,
	joinOnLeftKey bool,
	joinOnRightKey bool,
) error {
	/* SOLUTION {{{ */
	lSub := RepartitionEntries(lEntries)
	rSub := RepartitionEntries(rEntries)
	for i := 0; i < NUM_SUBPARTITIONS; i++ {
		if err := compareAndEmit(ctx, resultsChan, lSub[i], rSub[i], joinOnLeftKey, joinOnRightKey); err != nil {
			return err
		}
	}
	return nil
	/* SOLUTION }}} */
}

// ProbeBuckets sends the pair of entries in lBucket and rBucket that match on resultsChan after
// ensuring the total entries fits within JOIN_MEMORY_SIZE.
//
// The joinOnLeftKey and joinOnRightKey arguments dictate whether we originally were matching on keys
// or values for each bucket. For example, with joinOnLeftKey = true and joinOnRightKey = false,
// we are finding the entries in lBucket whose keys match the value of entries in rBucket.
func ProbeBuckets(
	ctx context.Context,
	resultsChan chan EntryPair,
	lBucket *hash.HashBucket,
	rBucket *hash.HashBucket,
	joinOnLeftKey bool,
	joinOnRightKey bool,
) error {
	defer lBucket.GetPage().GetPager().PutPage(lBucket.GetPage())
	defer rBucket.GetPage().GetPager().PutPage(rBucket.GetPage())
	/* SOLUTION {{{ */
	// Probe buckets.
	lEntries, err := lBucket.Select()
	if err != nil {
		return err
	}
	rEntries, err := rBucket.Select()
	if err != nil {
		return err
	}
	if int64(len(lEntries))+int64(len(rEntries)) <= JOIN_MEMORY_SIZE {
		return compareAndEmit(ctx, resultsChan, lEntries, rEntries, joinOnLeftKey, joinOnRightKey)
	}
	return RepartitionAndProbe(ctx, resultsChan, lEntries, rEntries, joinOnLeftKey, joinOnRightKey)
	/* SOLUTION }}} */
}

// ComputeBucketPairs returns the distinct pairs of bucket page numbers that must be probed against each other
// in order to compute the join
//
// Since we used the same hash function, if there was a value in the left table that matched with a value in the
// right table, they would be in the same bucket pair. To avoid outputting duplicate join results, keep track of
// all the pairs of buckets we have seen to be sure that we never process the same pair of buckets twice.
func ComputeBucketPairs(leftHashTable *hash.HashTable, rightHashTable *hash.HashTable) []Pair {
	/* SOLUTION {{{ */
	leftBuckets := leftHashTable.GetBuckets()
	rightBuckets := rightHashTable.GetBuckets()
	seenList := make(map[Pair]bool)
	bucketPairs := make([]Pair, 0)
	// Iterate through hash buckets, keeping track of pairs we've seen before.
	for i, lBucketPN := range leftBuckets {
		rBucketPN := rightBuckets[i]
		bucketPair := Pair{L: lBucketPN, R: rBucketPN}
		if _, seen := seenList[bucketPair]; seen {
			continue
		}
		seenList[bucketPair] = true
		bucketPairs = append(bucketPairs, bucketPair)
	}
	return bucketPairs
	/* SOLUTION }}} */
}

// Join leftIndex on rightIndex using the Grace Hash Join algorithm.
//
// The joinOnLeftKey and joinOnRightKey arguments dictate whether we are using keys or values
// for each bucket. For example, with joinOnLeftKey = true and joinOnRightKey = false,
// we are finding the entries in lBucket whose keys match the value of entries in rBucket.
func Join(
	ctx context.Context,
	leftIndex database.Index,
	rightIndex database.Index,
	joinOnLeftKey bool,
	joinOnRightKey bool,
) (resultsChan chan EntryPair, ctxt context.Context, group *errgroup.Group, cleanupCallback func(), err error) {
	// Create temporary hash tables for both tables
	leftHashIndex, leftDbName, err := buildHashIndex(leftIndex, joinOnLeftKey)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	rightHashIndex, rightDbName, err := buildHashIndex(rightIndex, joinOnRightKey)
	if err != nil {
		closeTempHashTable(leftHashIndex, leftDbName)
		return nil, nil, nil, nil, err
	}
	cleanupCallback = func() {
		closeTempHashTable(leftHashIndex, leftDbName)
		closeTempHashTable(rightHashIndex, rightDbName)
	}
	// Make both hash indices the same global size.
	leftHashTable := leftHashIndex.GetTable()
	rightHashTable := rightHashIndex.GetTable()
	for leftHashTable.GetDepth() != rightHashTable.GetDepth() {
		if leftHashTable.GetDepth() < rightHashTable.GetDepth() {
			// Split the left table
			leftHashTable.ExtendTable()
		} else {
			// Split the right table
			rightHashTable.ExtendTable()
		}
	}
	// Probe phase: match buckets to buckets and emit entries that match.
	group, ctx = errgroup.WithContext(ctx)
	resultsChan = make(chan EntryPair, 1024)
	bucketPairs := ComputeBucketPairs(leftHashTable, rightHashTable)
	for _, pair := range bucketPairs {
		lBucket, err := leftHashTable.GetBucketByPN(pair.L)
		if err != nil {
			return nil, nil, nil, cleanupCallback, err
		}
		rBucket, err := rightHashTable.GetBucketByPN(pair.R)
		if err != nil {
			lBucket.GetPage().GetPager().PutPage(lBucket.GetPage())
			return nil, nil, nil, cleanupCallback, err
		}
		group.Go(func() error {
			return ProbeBuckets(ctx, resultsChan, lBucket, rBucket, joinOnLeftKey, joinOnRightKey)
		})
	}
	return resultsChan, ctx, group, cleanupCallback, nil
}
