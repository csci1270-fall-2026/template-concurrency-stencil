package list

import (
	"fmt"
	"strconv"
)

// Define Str for List generic implementation
type Str string

func (s Str) String() string {
	return string(s)
}

// Define Int for List generic implementaiton

type Int int

func (i Int) String() string {
	return strconv.Itoa(int(i))
}

// List struct.
type List[T fmt.Stringer] struct {
	head *Link[T]
	tail *Link[T]
}

// Create a new list.
func NewList[T fmt.Stringer]() *List[T] {
	return &List[T]{nil, nil}
}

// Get a pointer to the head of the list.
func (list *List[T]) PeekHead() *Link[T] {
	return list.head
}

// Get a pointer to the tail of the list.
func (list *List[T]) PeekTail() *Link[T] {
	return list.tail
}

// Add an element to the start of the list. Returns the added link.
func (list *List[T]) PushHead(value T) *Link[T] {
	newlink := &Link[T]{list: list, prev: nil, next: list.head, value: value}
	if list.head != nil {
		list.head.prev = newlink
	}
	list.head = newlink
	if list.tail == nil {
		list.tail = newlink
	}
	return newlink
}

// Add an element to the end of the list. Returns the added link.
func (list *List[T]) PushTail(value T) *Link[T] {
	newlink := &Link[T]{list: list, prev: list.tail, next: nil, value: value}
	if list.tail != nil {
		list.tail.next = newlink
	}
	list.tail = newlink
	if list.head == nil {
		list.head = newlink
	}
	return newlink
}

// Find an element in a list given a boolean function, f, that evaluates to true on the desired element.
func (list *List[T]) Find(f func(*Link[T]) bool) *Link[T] {
	curr := list.head
	for curr != nil {
		if f(curr) {
			return curr
		}
		curr = curr.next
	}
	return nil
}

// Apply a function to every element in the list.
func (list *List[T]) Map(f func(*Link[T])) {
	curr := list.head
	for curr != nil {
		f(curr)
		curr = curr.next
	}
}

// Link struct.
type Link[T fmt.Stringer] struct {
	list  *List[T]
	prev  *Link[T]
	next  *Link[T]
	value T
}

// Get the list that this link is a part of.
func (link *Link[T]) GetList() *List[T] {
	return link.list
}

// Get the link's value.
func (link *Link[T]) GetValue() T {
	return link.value
}

// Set the link's value.
func (link *Link[T]) SetValue(value T) {
	link.value = value
}

// Get the link's prev.
func (link *Link[T]) GetPrev() *Link[T] {
	return link.prev
}

// Get the link's next.
func (link *Link[T]) GetNext() *Link[T] {
	return link.next
}

// Remove the link that calls PopSelf() from its list.
func (link *Link[T]) PopSelf() {
	if link.list == nil {
		return
	}

	if link.prev == nil && link.next == nil {
		// Only element in the list
		link.list.head = nil
		link.list.tail = nil
	} else if link.prev == nil {
		// Head of the list
		link.next.prev = nil
		link.list.head = link.next
	} else if link.next == nil {
		// Tail of the list
		link.prev.next = nil
		link.list.tail = link.prev
	} else {
		// Middle of the list
		link.prev.next = link.next
		link.next.prev = link.prev
	}

	link.list = nil
	link.prev = nil
	link.next = nil
}
