package join_test

import (
	"dinodb/pkg/join"
	"testing"
)

func TestBloomFilter(t *testing.T) {
	t.Run("Create", TestFilterCreate)
	t.Run("InsertAndCheckSmall", TestFilterInsertAndCheckSmall)
	t.Run("SizeSmall", TestFilterSizeSmall)
}

func TestFilterCreate(t *testing.T) {
	filter := join.CreateFilter(16)
	for i := 0; i < 1000; i++ {
		if filter.Contains(int64(i)) {
			t.Error("new filter should be empty")
		}
	}
}

func TestFilterInsertAndCheckSmall(t *testing.T) {
	filter := join.CreateFilter(16)
	for i := 0; i < 10; i++ {
		filter.Insert(int64(i))
		if !filter.Contains(int64(i)) {
			t.Errorf("inserted value %d but not found", i)
		}
	}
}

func TestFilterSizeSmall(t *testing.T) {
	filter := join.CreateFilter(1)
	filter.Insert(int64(0))
	for i := 0; i < 100; i++ {
		if !filter.Contains(int64(i)) {
			t.Errorf("should have 'found' value %d", i)
		}
	}
}
