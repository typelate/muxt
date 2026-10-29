// Ported from lib/utils/List.js.

package csstree

import (
	"encoding/json"
	"errors"
	"iter"
)

//
//                              list
//                            ┌──────┐
//             ┌──────────────┼─head │
//             │              │ tail─┼──────────────┐
//             │              └──────┘              │
//             ▼                                    ▼
//            item        item        item        item
//          ┌──────┐    ┌──────┐    ┌──────┐    ┌──────┐
//  null ◀──┼─prev │◀───┼─prev │◀───┼─prev │◀───┼─prev │
//          │ next─┼───▶│ next─┼───▶│ next─┼───▶│ next─┼──▶ null
//          ├──────┤    ├──────┤    ├──────┤    ├──────┤
//          │ data │    │ data │    │ data │    │ data │
//          └──────┘    └──────┘    └──────┘    └──────┘
//

var (
	errBeforeNotInList = errors.New("before doesn't belong to list")
	errItemNotInList   = errors.New("item doesn't belong to list")
)

// ListItem is one link of a List. Prev and Next are nil at the ends.
type ListItem[T any] struct {
	Prev *ListItem[T]
	Next *ListItem[T]
	Data T
}

// listCursor records where an in-progress iteration continues, so that
// items can be inserted or removed while the list is being iterated.
// Cursors of nested iterations form a stack through the cursor field.
type listCursor[T any] struct {
	prev   *ListItem[T]
	next   *ListItem[T]
	cursor *listCursor[T]
}

// List is a doubly linked list whose iteration methods (ForEach,
// ForEachRight, Reduce, ReduceRight, NextUntil, PrevUntil) tolerate the
// callback inserting or removing items of the list being iterated.
//
// The zero value is an empty list ready to use. A List must not be used by
// more than one goroutine at a time: even the iteration methods update the
// list's internal cursor stack.
type List[T any] struct {
	head   *ListItem[T]
	tail   *ListItem[T]
	cursor *listCursor[T]

	// releasedCursors is a free list of cursors for reuse.
	releasedCursors *listCursor[T]
}

// NewListItem returns an unlinked item holding data (upstream List.createItem).
//
// Type inference picks T from data, so for a List[Node] holding an
// *Identifier use list.CreateItem or NewListItem[Node].
func NewListItem[T any](data T) *ListItem[T] {
	return &ListItem[T]{
		Prev: nil,
		Next: nil,
		Data: data,
	}
}

// CreateItem returns an unlinked item holding data, typed for l.
func (l *List[T]) CreateItem(data T) *ListItem[T] {
	return NewListItem(data)
}

// cursor helpers
func (l *List[T]) allocateCursor(prev, next *ListItem[T]) *listCursor[T] {
	var cursor *listCursor[T]

	if l.releasedCursors != nil {
		cursor = l.releasedCursors
		l.releasedCursors = l.releasedCursors.cursor
		cursor.prev = prev
		cursor.next = next
		cursor.cursor = l.cursor
	} else {
		cursor = &listCursor[T]{
			prev:   prev,
			next:   next,
			cursor: l.cursor,
		}
	}

	l.cursor = cursor

	return cursor
}

func (l *List[T]) releaseCursor() {
	cursor := l.cursor

	l.cursor = cursor.cursor
	cursor.prev = nil
	cursor.next = nil
	cursor.cursor = l.releasedCursors
	l.releasedCursors = cursor
}

func (l *List[T]) updateCursors(prevOld, prevNew, nextOld, nextNew *ListItem[T]) {
	cursor := l.cursor

	for cursor != nil {
		if cursor.prev == prevOld {
			cursor.prev = prevNew
		}

		if cursor.next == nextOld {
			cursor.next = nextNew
		}

		cursor = cursor.cursor
	}
}

// All returns an iterator over the data of l from head to tail
// (upstream [Symbol.iterator]). Unlike ForEach it does not track
// mutation: removing the current item ends the iteration.
func (l *List[T]) All() iter.Seq[T] {
	return func(yield func(T) bool) {
		for cursor := l.head; cursor != nil; cursor = cursor.Next {
			if !yield(cursor.Data) {
				return
			}
		}
	}
}

// Head returns the first item, or nil when l is empty.
func (l *List[T]) Head() *ListItem[T] {
	return l.head
}

// Tail returns the last item, or nil when l is empty.
func (l *List[T]) Tail() *ListItem[T] {
	return l.tail
}

// getters

// Size counts the items of l.
func (l *List[T]) Size() int {
	size := 0

	for cursor := l.head; cursor != nil; cursor = cursor.Next {
		size++
	}

	return size
}

// IsEmpty reports whether l has no items.
func (l *List[T]) IsEmpty() bool {
	return l.head == nil
}

// First returns the data of the head item, or the zero value of T (nil for
// List[Node]) when l is empty.
func (l *List[T]) First() T {
	var data T
	if l.head != nil {
		data = l.head.Data
	}
	return data
}

// Last returns the data of the tail item, or the zero value of T (nil for
// List[Node]) when l is empty.
func (l *List[T]) Last() T {
	var data T
	if l.tail != nil {
		data = l.tail.Data
	}
	return data
}

// convertors

// FromArray replaces the items of l with new items holding array's
// elements, and returns l.
func (l *List[T]) FromArray(array []T) *List[T] {
	var cursor *ListItem[T]
	l.head = nil

	for _, data := range array {
		item := NewListItem(data)

		if cursor != nil {
			cursor.Next = item
		} else {
			l.head = item
		}

		item.Prev = cursor
		cursor = item
	}

	l.tail = cursor
	return l
}

// ToArray returns the data of l from head to tail. The result is non-nil
// even when l is empty.
func (l *List[T]) ToArray() []T {
	array := []T{}
	for data := range l.All() {
		array = append(array, data)
	}
	return array
}

// MarshalJSON encodes l as a JSON array of its data (upstream toJSON).
func (l *List[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(l.ToArray())
}

// array-like methods

// ForEach calls fn for each item from head to tail. fn may insert or remove
// items of list: items removed before they are reached are skipped and
// items inserted ahead of the current one are visited.
func (l *List[T]) ForEach(fn func(data T, item *ListItem[T], list *List[T])) {
	// push cursor
	cursor := l.allocateCursor(nil, l.head)

	for cursor.next != nil {
		item := cursor.next
		cursor.next = item.Next
		fn(item.Data, item, l)
	}

	// pop cursor
	l.releaseCursor()
}

// ForEachRight is ForEach from tail to head.
func (l *List[T]) ForEachRight(fn func(data T, item *ListItem[T], list *List[T])) {
	// push cursor
	cursor := l.allocateCursor(l.tail, nil)

	for cursor.prev != nil {
		item := cursor.prev
		cursor.prev = item.Prev
		fn(item.Data, item, l)
	}

	// pop cursor
	l.releaseCursor()
}

// Reduce folds l from head to tail, starting from initialValue, with the
// same tolerance of mutation as ForEach. It is a function rather than a
// method because Go methods cannot add the type parameter R.
func Reduce[T, R any](l *List[T], fn func(acc R, data T, item *ListItem[T], list *List[T]) R, initialValue R) R {
	// push cursor
	cursor := l.allocateCursor(nil, l.head)
	acc := initialValue
	var item *ListItem[T]

	for cursor.next != nil {
		item = cursor.next
		cursor.next = item.Next

		acc = fn(acc, item.Data, item, l)
	}

	// pop cursor
	l.releaseCursor()

	return acc
}

// ReduceRight is Reduce from tail to head.
func ReduceRight[T, R any](l *List[T], fn func(acc R, data T, item *ListItem[T], list *List[T]) R, initialValue R) R {
	// push cursor
	cursor := l.allocateCursor(l.tail, nil)
	acc := initialValue
	var item *ListItem[T]

	for cursor.prev != nil {
		item = cursor.prev
		cursor.prev = item.Prev

		acc = fn(acc, item.Data, item, l)
	}

	// pop cursor
	l.releaseCursor()

	return acc
}

// Some reports whether fn returns true for any item, stopping at the first
// that does. fn must not mutate l.
func (l *List[T]) Some(fn func(data T, item *ListItem[T], list *List[T]) bool) bool {
	for cursor := l.head; cursor != nil; cursor = cursor.Next {
		if fn(cursor.Data, cursor, l) {
			return true
		}
	}

	return false
}

// Map returns a new list of the results of fn for each item. fn must not
// mutate l. The result has the same element type as l: upstream only maps
// nodes to nodes (clone), and Go methods cannot add a type parameter.
func (l *List[T]) Map(fn func(data T, item *ListItem[T], list *List[T]) T) *List[T] {
	result := new(List[T])

	for cursor := l.head; cursor != nil; cursor = cursor.Next {
		result.AppendData(fn(cursor.Data, cursor, l))
	}

	return result
}

// Filter returns a new list of the data for which fn returns true. fn must
// not mutate l.
func (l *List[T]) Filter(fn func(data T, item *ListItem[T], list *List[T]) bool) *List[T] {
	result := new(List[T])

	for cursor := l.head; cursor != nil; cursor = cursor.Next {
		if fn(cursor.Data, cursor, l) {
			result.AppendData(cursor.Data)
		}
	}

	return result
}

// NextUntil calls fn for start and each following item until fn returns
// true. It does nothing when start is nil. It tolerates mutation like
// ForEach.
func (l *List[T]) NextUntil(start *ListItem[T], fn func(data T, item *ListItem[T], list *List[T]) bool) {
	if start == nil {
		return
	}

	// push cursor
	cursor := l.allocateCursor(nil, start)

	for cursor.next != nil {
		item := cursor.next
		cursor.next = item.Next
		if fn(item.Data, item, l) {
			break
		}
	}

	// pop cursor
	l.releaseCursor()
}

// PrevUntil is NextUntil towards the head.
func (l *List[T]) PrevUntil(start *ListItem[T], fn func(data T, item *ListItem[T], list *List[T]) bool) {
	if start == nil {
		return
	}

	// push cursor
	cursor := l.allocateCursor(start, nil)

	for cursor.prev != nil {
		item := cursor.prev
		cursor.prev = item.Prev
		if fn(item.Data, item, l) {
			break
		}
	}

	// pop cursor
	l.releaseCursor()
}

// mutation

// Clear unlinks l from its items. The items keep their links to each other.
func (l *List[T]) Clear() {
	l.head = nil
	l.tail = nil
}

// Copy returns a new list with new items holding the same data.
func (l *List[T]) Copy() *List[T] {
	result := new(List[T])

	for data := range l.All() {
		result.AppendData(data)
	}

	return result
}

// Prepend links item before the head and returns l.
func (l *List[T]) Prepend(item *ListItem[T]) *List[T] {
	//      head
	//    ^
	// item
	l.updateCursors(nil, item, l.head, item)

	// insert to the beginning of the list
	if l.head != nil {
		// new item <- first item
		l.head.Prev = item
		// new item -> first item
		item.Next = l.head
	} else {
		// if list has no head, then it also has no tail
		// in this case tail points to the new item
		l.tail = item
	}

	// head always points to new item
	l.head = item
	return l
}

// PrependData links a new item holding data before the head and returns l.
func (l *List[T]) PrependData(data T) *List[T] {
	return l.Prepend(NewListItem(data))
}

// Append links item after the tail and returns l.
func (l *List[T]) Append(item *ListItem[T]) *List[T] {
	return l.Insert(item, nil)
}

// AppendData links a new item holding data after the tail and returns l.
func (l *List[T]) AppendData(data T) *List[T] {
	return l.Insert(NewListItem(data), nil)
}

// Insert links item before the item before, or after the tail when before
// is nil, and returns l.
//
// It panics if before is the first item of a different list; upstream
// throws for this programming error.
func (l *List[T]) Insert(item, before *ListItem[T]) *List[T] {
	if before != nil {
		// prev   before
		//      ^
		//     item
		l.updateCursors(before.Prev, item, before, item)

		if before.Prev == nil {
			// insert to the beginning of list
			if l.head != before {
				panic(errBeforeNotInList)
			}
			// since head points to before therefore list doesn't empty
			// no need to check tail
			l.head = item
			before.Prev = item
			item.Next = before
			// upstream passes undefined for nextOld and nextNew, which no
			// cursor matches; nil for both leaves next unchanged as well
			l.updateCursors(nil, item, nil, nil)
		} else {
			// insert between two items
			before.Prev.Next = item
			item.Prev = before.Prev
			before.Prev = item
			item.Next = before
		}
	} else {
		// tail
		//      ^
		//      item
		l.updateCursors(l.tail, item, nil, item)

		// insert to the ending of the list
		if l.tail != nil {
			// last item -> new item
			l.tail.Next = item
			// last item <- new item
			item.Prev = l.tail
		} else {
			// if list has no tail, then it also has no head
			// in this case head points to new item
			l.head = item
		}

		// tail always points to new item
		l.tail = item
	}

	return l
}

// InsertData links a new item holding data before the item before, or
// after the tail when before is nil, and returns l. It panics like Insert.
func (l *List[T]) InsertData(data T, before *ListItem[T]) *List[T] {
	return l.Insert(NewListItem(data), before)
}

// Remove unlinks item from l and returns it.
//
// It panics if item is the head or tail of a different list; upstream
// throws for this programming error.
func (l *List[T]) Remove(item *ListItem[T]) *ListItem[T] {
	//      item
	//       ^
	// prev     next
	l.updateCursors(item, item.Prev, item, item.Next)

	if item.Prev != nil {
		item.Prev.Next = item.Next
	} else {
		if l.head != item {
			panic(errItemNotInList)
		}

		l.head = item.Next
	}

	if item.Next != nil {
		item.Next.Prev = item.Prev
	} else {
		if l.tail != item {
			panic(errItemNotInList)
		}

		l.tail = item.Prev
	}

	item.Prev = nil
	item.Next = nil

	return item
}

// Push links a new item holding data after the tail.
func (l *List[T]) Push(data T) {
	l.Insert(NewListItem(data), nil)
}

// Pop unlinks and returns the tail item, or returns nil when l is empty.
func (l *List[T]) Pop() *ListItem[T] {
	if l.tail != nil {
		return l.Remove(l.tail)
	}
	return nil
}

// Unshift links a new item holding data before the head.
func (l *List[T]) Unshift(data T) {
	l.Prepend(NewListItem(data))
}

// Shift unlinks and returns the head item, or returns nil when l is empty.
func (l *List[T]) Shift() *ListItem[T] {
	if l.head != nil {
		return l.Remove(l.head)
	}
	return nil
}

// PrependList moves the items of list before the head of l, leaving list
// empty, and returns l.
func (l *List[T]) PrependList(list *List[T]) *List[T] {
	return l.InsertList(list, l.head)
}

// AppendList moves the items of list after the tail of l, leaving list
// empty, and returns l.
func (l *List[T]) AppendList(list *List[T]) *List[T] {
	return l.InsertList(list, nil)
}

// InsertList moves the items of list before the item before, or after the
// tail when before is nil, leaving list empty, and returns l.
func (l *List[T]) InsertList(list *List[T], before *ListItem[T]) *List[T] {
	// ignore empty lists
	if list.head == nil {
		return l
	}

	if before != nil {
		l.updateCursors(before.Prev, list.tail, before, list.head)

		// insert in the middle of dist list
		if before.Prev != nil {
			// before.prev <-> list.head
			before.Prev.Next = list.head
			list.head.Prev = before.Prev
		} else {
			l.head = list.head
		}

		before.Prev = list.tail
		list.tail.Next = before
	} else {
		l.updateCursors(l.tail, list.tail, nil, list.head)

		// insert to end of the list
		if l.tail != nil {
			// if destination list has a tail, then it also has a head,
			// but head doesn't change
			// dest tail -> source head
			l.tail.Next = list.head
			// dest tail <- source head
			list.head.Prev = l.tail
		} else {
			// if list has no a tail, then it also has no a head
			// in this case points head to new item
			l.head = list.head
		}

		// tail always start point to new item
		l.tail = list.tail
	}

	list.head = nil
	list.tail = nil
	return l
}

// Replace links newItem in place of oldItem and unlinks oldItem. Upstream
// replace takes an item or a list; ReplaceList takes the list.
func (l *List[T]) Replace(oldItem, newItem *ListItem[T]) {
	l.Insert(newItem, oldItem)

	l.Remove(oldItem)
}

// ReplaceList moves the items of list in place of oldItem, leaving list
// empty, and unlinks oldItem.
func (l *List[T]) ReplaceList(oldItem *ListItem[T], list *List[T]) {
	l.InsertList(list, oldItem)

	l.Remove(oldItem)
}
