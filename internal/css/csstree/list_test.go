// Ported from lib/__tests/list.js.

package csstree

import (
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"testing"
)

// Upstream uses two distinct empty objects as foo and bar and compares them
// by identity. Distinct strings give the same guarantees for a List[string]
// and let the modification tests mix them with the "ins<N>" strings they add.
const (
	foo = "foo"
	bar = "bar"
)

func getData(item *ListItem[string]) string {
	return item.Data
}

// iteratorState drops upstream's thisArg: Go callbacks are closures.
type iteratorState struct {
	data string
	item *ListItem[string]
	list *List[string]
}

func getIteratorState(data string, item *ListItem[string], list *List[string]) iteratorState {
	return iteratorState{
		data: data,
		item: item,
		list: list,
	}
}

type iteratorStateCollector struct {
	states   []iteratorState
	retValue bool
}

func (c *iteratorStateCollector) collect(data string, item *ListItem[string], list *List[string]) bool {
	c.states = append(c.states, getIteratorState(data, item, list))
	return c.retValue
}

func createIteratorStateCollector(retValue bool) *iteratorStateCollector {
	return &iteratorStateCollector{retValue: retValue}
}

// toArray checks every link of list before returning its data.
func toArray(t *testing.T, list *List[string]) []string {
	t.Helper()

	cursor := list.Head()
	var prev *ListItem[string]

	for cursor != nil {
		if cursor.Prev != prev {
			t.Fatal("Wrong prev reference")
		}

		if prev != nil && prev.Next != cursor {
			t.Fatal("Wrong next reference")
		}

		prev = cursor
		cursor = cursor.Next
	}

	if list.Tail() != prev {
		t.Fatal("Wrong tail reference")
	}

	if prev != nil && prev.Next != nil {
		t.Fatal("Wrong next reference")
	}

	return list.ToArray()
}

// iterateFunc adapts one List iteration method to a common shape so the
// shared iterator tests can drive it, like upstream's list[method].bind(...).
// until is the start item for nextUntil and prevUntil and ignored otherwise.
type iterateFunc func(list *List[string], until *ListItem[string], fn func(data string, item *ListItem[string], list *List[string]) bool)

var iterateMethods = map[string]iterateFunc{
	"forEach": func(list *List[string], _ *ListItem[string], fn func(string, *ListItem[string], *List[string]) bool) {
		list.ForEach(func(data string, item *ListItem[string], list *List[string]) {
			fn(data, item, list)
		})
	},
	"forEachRight": func(list *List[string], _ *ListItem[string], fn func(string, *ListItem[string], *List[string]) bool) {
		list.ForEachRight(func(data string, item *ListItem[string], list *List[string]) {
			fn(data, item, list)
		})
	},
	"nextUntil": func(list *List[string], until *ListItem[string], fn func(string, *ListItem[string], *List[string]) bool) {
		list.NextUntil(until, fn)
	},
	"prevUntil": func(list *List[string], until *ListItem[string], fn func(string, *ListItem[string], *List[string]) bool) {
		list.PrevUntil(until, fn)
	},
	"some": func(list *List[string], _ *ListItem[string], fn func(string, *ListItem[string], *List[string]) bool) {
		list.Some(fn)
	},
	"map": func(list *List[string], _ *ListItem[string], fn func(string, *ListItem[string], *List[string]) bool) {
		list.Map(func(data string, item *ListItem[string], list *List[string]) string {
			fn(data, item, list)
			return data
		})
	},
	"filter": func(list *List[string], _ *ListItem[string], fn func(string, *ListItem[string], *List[string]) bool) {
		list.Filter(fn)
	},
}

func createIteratorTests(t *testing.T, list *List[string], method string, items []*ListItem[string], until *ListItem[string], retValue bool) {
	iterate := func(fn func(string, *ListItem[string], *List[string]) bool) {
		iterateMethods[method](list, until, fn)
	}

	t.Run("iterate", func(t *testing.T) {
		collector := createIteratorStateCollector(retValue)

		iterate(collector.collect)
		var want []iteratorState
		for _, item := range items {
			want = append(want, getIteratorState(item.Data, item, list))
		}
		if !slices.Equal(collector.states, want) {
			t.Errorf("%s visited %+v, want %+v", method, collector.states, want)
		}
	})

	t.Run("iterate with thisArg", func(t *testing.T) {
		t.Skip("not ported: Go callbacks are closures, there is no thisArg to bind")
	})

	t.Run("nested iterate", func(t *testing.T) {
		collector := createIteratorStateCollector(retValue)

		iterate(func(string, *ListItem[string], *List[string]) bool {
			iterate(collector.collect)
			return retValue
		})

		var want []iteratorState
		for range items {
			for _, item := range items {
				want = append(want, getIteratorState(item.Data, item, list))
			}
		}
		if !slices.Equal(collector.states, want) {
			t.Errorf("nested %s visited %+v, want %+v", method, collector.states, want)
		}
	})
}

var insertedData = regexp.MustCompile(`^ins\d+$`)

func createIteratorWithModificationTests(t *testing.T, list *List[string], method string, items []*ListItem[string], until *ListItem[string], retValue bool) {
	iterate := func(list *List[string], fn func(string, *ListItem[string], *List[string]) bool) {
		var untilCopy *ListItem[string]
		if until != nil {
			cursor := list.Head()

			for cursor != nil {
				if cursor.Data == until.Data {
					untilCopy = cursor
					break
				}

				cursor = cursor.Next
			}
		}

		iterateMethods[method](list, untilCopy, fn)
	}

	t.Run("remove items on iterate", func(t *testing.T) {
		iterateList := list.Copy()
		var removed []string

		iterate(iterateList, func(data string, item *ListItem[string], _ *List[string]) bool {
			removed = append(removed, data)
			iterateList.Remove(item)
			return retValue
		})

		if iterateList.Head() != nil {
			t.Errorf("Head() = %+v after removing every item, want nil", iterateList.Head())
		}
		var want []string
		for _, item := range items {
			want = append(want, getData(item))
		}
		if !slices.Equal(removed, want) {
			t.Errorf("removed %q, want %q", removed, want)
		}
	})

	t.Run("insert items on iterate", func(t *testing.T) {
		iterateList := list.Copy()
		var inserted []string
		var order []string
		idx := 0

		iterate(iterateList, func(data string, item *ListItem[string], _ *List[string]) bool {
			if !insertedData.MatchString(data) {
				before := NewListItem("ins" + strconv.Itoa(idx))
				after := NewListItem("ins" + strconv.Itoa(idx))

				iterateList.Insert(before, item)
				iterateList.Insert(after, item.Next)
				order = append(order, data)
				inserted = append(inserted, data, "ins"+strconv.Itoa(idx))
				idx++
			}

			return retValue
		})

		var wantInserted []string
		for idx, item := range items {
			wantInserted = append(wantInserted, item.Data, "ins"+strconv.Itoa(idx))
		}
		if !slices.Equal(inserted, wantInserted) {
			t.Errorf("inserted %q, want %q", inserted, wantInserted)
		}

		var want []string
		for data := range list.All() {
			idx := slices.Index(order, data)
			want = append(want, "ins"+strconv.Itoa(idx), data, "ins"+strconv.Itoa(idx))
		}
		if got := slices.Collect(iterateList.All()); !slices.Equal(got, want) {
			t.Errorf("list after insertions = %q, want %q", got, want)
		}
	})
}

// assertItem replaces upstream's deepStrictEqual(item, {prev, next, data}).
func assertItem(t *testing.T, name string, item, prev, next *ListItem[string], data string) {
	t.Helper()
	if item.Prev != prev || item.Next != next || item.Data != data {
		t.Errorf("%s = {Prev: %p, Next: %p, Data: %q}, want {Prev: %p, Next: %p, Data: %q}",
			name, item.Prev, item.Next, item.Data, prev, next, data)
	}
}

// assertPanicsWith replaces upstream's assert.throws for misuse that throws.
func assertPanicsWith(t *testing.T, want error, fn func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		err, ok := r.(error)
		if !ok || !errors.Is(err, want) {
			t.Errorf("recovered %v, want panic with %v", r, want)
		}
	}()
	fn()
}

func TestList(t *testing.T) {
	var (
		empty *List[string]
		list1 *List[string]
		list2 *List[string]
	)

	// run is upstream's it() with its beforeEach.
	run := func(t *testing.T, name string, fn func(t *testing.T)) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			empty = new(List[string])
			list1 = new(List[string]).FromArray([]string{foo})
			list2 = new(List[string]).FromArray([]string{foo, bar})
			fn(t)
		})
	}

	run(t, ".createItem()", func(t *testing.T) {
		if got, want := *NewListItem(foo), (ListItem[string]{Prev: nil, Next: nil, Data: foo}); got != want {
			t.Errorf("NewListItem(%q) = %+v, want %+v", foo, got, want)
		}
	})

	run(t, "#createItem()", func(t *testing.T) {
		if got, want := *empty.CreateItem(foo), (ListItem[string]{Prev: nil, Next: nil, Data: foo}); got != want {
			t.Errorf("CreateItem(%q) = %+v, want %+v", foo, got, want)
		}
	})

	run(t, "#size", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			list *List[string]
			want int
		}{
			{"empty", empty, 0},
			{"list1", list1, 1},
			{"list2", list2, 2},
		} {
			if got := tt.list.Size(); got != tt.want {
				t.Errorf("%s.Size() = %d, want %d", tt.name, got, tt.want)
			}
		}
	})

	run(t, "#fromArray()", func(t *testing.T) {
		list := new(List[string]).FromArray([]string{foo, bar})

		if got := list.Head().Data; got != foo {
			t.Errorf("Head().Data = %q, want %q", got, foo)
		}
		if got := list.Tail().Data; got != bar {
			t.Errorf("Tail().Data = %q, want %q", got, bar)
		}
	})

	run(t, "#toArray()", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			list *List[string]
			want []string
		}{
			{"empty", empty, []string{}},
			{"list1", list1, []string{foo}},
			{"list2", list2, []string{foo, bar}},
		} {
			if got := tt.list.ToArray(); !slices.Equal(got, tt.want) {
				t.Errorf("%s.ToArray() = %q, want %q", tt.name, got, tt.want)
			}
		}
	})

	run(t, "#toJSON()", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			list *List[string]
			want string
		}{
			{"empty", empty, `[]`},
			{"list1", list1, `["foo"]`},
			{"list2", list2, `["foo","bar"]`},
		} {
			got, err := json.Marshal(tt.list)
			if err != nil {
				t.Fatalf("json.Marshal(%s) error: %v", tt.name, err)
			}
			if string(got) != tt.want {
				t.Errorf("json.Marshal(%s) = %s, want %s", tt.name, got, tt.want)
			}
		}
	})

	run(t, "#isEmpty", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			list *List[string]
			want bool
		}{
			{"empty", empty, true},
			{"list1", list1, false},
			{"list2", list2, false},
		} {
			if got := tt.list.IsEmpty(); got != tt.want {
				t.Errorf("%s.IsEmpty() = %t, want %t", tt.name, got, tt.want)
			}
		}
	})

	run(t, "#first", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			list *List[string]
			want string
		}{
			{"empty", empty, ""}, // upstream: null
			{"list1", list1, foo},
			{"list2", list2, foo},
		} {
			if got := tt.list.First(); got != tt.want {
				t.Errorf("%s.First() = %q, want %q", tt.name, got, tt.want)
			}
		}
	})

	run(t, "#last", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			list *List[string]
			want string
		}{
			{"empty", empty, ""}, // upstream: null
			{"list1", list1, foo},
			{"list2", list2, bar},
		} {
			if got := tt.list.Last(); got != tt.want {
				t.Errorf("%s.Last() = %q, want %q", tt.name, got, tt.want)
			}
		}
	})

	t.Run("#forEach()", func(t *testing.T) {
		iterateList := new(List[string]).FromArray([]string{foo, bar})
		createIteratorTests(t, iterateList, "forEach", []*ListItem[string]{iterateList.Head(), iterateList.Tail()}, nil, false)
		createIteratorWithModificationTests(t, iterateList, "forEach", []*ListItem[string]{iterateList.Head(), iterateList.Tail()}, nil, false)
	})

	t.Run("#forEachRight()", func(t *testing.T) {
		iterateList := new(List[string]).FromArray([]string{foo, bar})
		createIteratorTests(t, iterateList, "forEachRight", []*ListItem[string]{iterateList.Tail(), iterateList.Head()}, nil, false)
		createIteratorWithModificationTests(t, iterateList, "forEachRight", []*ListItem[string]{iterateList.Tail(), iterateList.Head()}, nil, false)
	})

	run(t, "#reduce()", func(t *testing.T) {
		expected := "foobar"
		actual := Reduce(new(List[string]).FromArray([]string{"foo", "bar"}), func(res string, data string, _ *ListItem[string], _ *List[string]) string {
			return res + data
		}, "")

		if actual != expected {
			t.Errorf("Reduce(...) = %q, want %q", actual, expected)
		}
	})

	run(t, "#reduceRight()", func(t *testing.T) {
		expected := "barfoo"
		actual := ReduceRight(new(List[string]).FromArray([]string{"foo", "bar"}), func(res string, data string, _ *ListItem[string], _ *List[string]) string {
			return res + data
		}, "")

		if actual != expected {
			t.Errorf("ReduceRight(...) = %q, want %q", actual, expected)
		}
	})

	t.Run("#nextUntil()", func(t *testing.T) {
		run(t, "should not iterate when start is null", func(t *testing.T) {
			count := 0

			list2.NextUntil(nil, func(string, *ListItem[string], *List[string]) bool {
				count++
				return false
			})

			if count != 0 {
				t.Errorf("callback called %d times, want 0", count)
			}
		})

		run(t, "should stop iterate when callback returns true", func(t *testing.T) {
			count := 0

			list2.NextUntil(list2.Head(), func(string, *ListItem[string], *List[string]) bool {
				count++
				return true
			})

			if count != 1 {
				t.Errorf("callback called %d times, want 1", count)
			}
		})

		iterateList := new(List[string]).FromArray([]string{foo, bar})
		createIteratorTests(t, iterateList, "nextUntil", []*ListItem[string]{iterateList.Head(), iterateList.Tail()}, iterateList.Head(), false)
		createIteratorWithModificationTests(t, iterateList, "nextUntil", []*ListItem[string]{iterateList.Head(), iterateList.Tail()}, iterateList.Head(), false)
	})

	t.Run("#prevUntil()", func(t *testing.T) {
		run(t, "should not iterate when start is null", func(t *testing.T) {
			count := 0

			list2.PrevUntil(nil, func(string, *ListItem[string], *List[string]) bool {
				count++
				return false
			})

			if count != 0 {
				t.Errorf("callback called %d times, want 0", count)
			}
		})

		run(t, "should stop iterate when callback returns true", func(t *testing.T) {
			count := 0

			list2.PrevUntil(list2.Tail(), func(string, *ListItem[string], *List[string]) bool {
				count++
				return true
			})

			if count != 1 {
				t.Errorf("callback called %d times, want 1", count)
			}
		})

		iterateList := new(List[string]).FromArray([]string{foo, bar})
		createIteratorTests(t, iterateList, "prevUntil", []*ListItem[string]{iterateList.Tail(), iterateList.Head()}, iterateList.Tail(), false)
		createIteratorWithModificationTests(t, iterateList, "prevUntil", []*ListItem[string]{iterateList.Tail(), iterateList.Head()}, iterateList.Tail(), false)
	})

	// truthy stands in for upstream's Boolean: every test datum is a
	// non-empty string, as every upstream datum is an object.
	truthy := func(data string, _ *ListItem[string], _ *List[string]) bool {
		return data != ""
	}

	t.Run("#some()", func(t *testing.T) {
		run(t, "basic", func(t *testing.T) {
			if got := list2.Some(truthy); got != true {
				t.Errorf("Some(truthy) = %t, want true", got)
			}
			if got := list2.Some(func(string, *ListItem[string], *List[string]) bool { return false }); got != false {
				t.Errorf("Some(always false) = %t, want false", got)
			}
			if got := list2.Some(func(data string, _ *ListItem[string], _ *List[string]) bool {
				return data == bar
			}); got != true {
				t.Errorf("Some(data == bar) = %t, want true", got)
			}
		})

		run(t, "should stop on first match", func(t *testing.T) {
			count := 0
			if got := list2.Some(func(data string, _ *ListItem[string], _ *List[string]) bool {
				count++
				return data == foo
			}); got != true {
				t.Errorf("Some(data == foo) = %t, want true", got)
			}
			if count != 1 {
				t.Errorf("callback called %d times, want 1", count)
			}
		})

		iterateList := new(List[string]).FromArray([]string{foo, bar})
		createIteratorTests(t, iterateList, "some", []*ListItem[string]{iterateList.Head(), iterateList.Tail()}, nil, false)
	})

	t.Run("#map()", func(t *testing.T) {
		getFirstArg := func(data string, _ *ListItem[string], _ *List[string]) string {
			return data
		}

		run(t, "basic", func(t *testing.T) {
			if got := empty.Map(getFirstArg).ToArray(); !slices.Equal(got, []string{}) {
				t.Errorf("empty.Map(getFirstArg) = %q, want []", got)
			}
			if got := list1.Map(getFirstArg).ToArray(); !slices.Equal(got, []string{foo}) {
				t.Errorf("list1.Map(getFirstArg) = %q, want %q", got, []string{foo})
			}

			const (
				foo2 = "foo2"
				bar2 = "bar2"
			)
			mapped := list2.Map(func(node string, _ *ListItem[string], _ *List[string]) string {
				if node == foo {
					return foo2
				}
				return bar2
			})
			if got, want := mapped.ToArray(), []string{foo2, bar2}; !slices.Equal(got, want) {
				t.Errorf("mapped = %q, want %q", got, want)
			}
			if got, want := list2.ToArray(), []string{foo, bar}; !slices.Equal(got, want) {
				t.Errorf("list2 after Map = %q, want %q", got, want)
			}
		})

		iterateList := new(List[string]).FromArray([]string{foo, bar})
		createIteratorTests(t, iterateList, "map", []*ListItem[string]{iterateList.Head(), iterateList.Tail()}, nil, false)
	})

	t.Run("#filter()", func(t *testing.T) {
		run(t, "basic", func(t *testing.T) {
			// upstream filters the empty list with getFirstArg; any predicate will do
			if got := empty.Filter(truthy).ToArray(); !slices.Equal(got, []string{}) {
				t.Errorf("empty.Filter(truthy) = %q, want []", got)
			}
			if got, want := list2.Filter(truthy).ToArray(), []string{foo, bar}; !slices.Equal(got, want) {
				t.Errorf("list2.Filter(truthy) = %q, want %q", got, want)
			}

			filtered := list2.Filter(func(node string, _ *ListItem[string], _ *List[string]) bool {
				return node == bar
			})
			if got, want := filtered.ToArray(), []string{bar}; !slices.Equal(got, want) {
				t.Errorf("filtered = %q, want %q", got, want)
			}
			if got, want := list2.ToArray(), []string{foo, bar}; !slices.Equal(got, want) {
				t.Errorf("list2 after Filter = %q, want %q", got, want)
			}
		})

		iterateList := new(List[string]).FromArray([]string{foo, bar})
		createIteratorTests(t, iterateList, "filter", []*ListItem[string]{iterateList.Head(), iterateList.Tail()}, nil, false)
	})

	t.Run("#clear()", func(t *testing.T) {
		run(t, "empty list", func(t *testing.T) {
			if empty.Head() != nil || empty.Tail() != nil {
				t.Fatalf("empty list has Head() = %p, Tail() = %p, want nil", empty.Head(), empty.Tail())
			}
			empty.Clear()
			if empty.Head() != nil || empty.Tail() != nil {
				t.Errorf("after Clear() Head() = %p, Tail() = %p, want nil", empty.Head(), empty.Tail())
			}
		})

		run(t, "non-empty list", func(t *testing.T) {
			if list2.Head() == nil || list2.Tail() == nil {
				t.Fatalf("list2 has Head() = %p, Tail() = %p, want non-nil", list2.Head(), list2.Tail())
			}
			list2.Clear()
			if list2.Head() != nil || list2.Tail() != nil {
				t.Errorf("after Clear() Head() = %p, Tail() = %p, want nil", list2.Head(), list2.Tail())
			}
		})
	})

	run(t, "#copy()", func(t *testing.T) {
		copied := list2.Copy()

		if copied == list2 {
			t.Error("Copy() returned the same list")
		}
		if copied.Head() == list2.Head() {
			t.Error("Copy().Head() is the same item as Head()")
		}
		if copied.Head().Data != list2.Head().Data {
			t.Errorf("Copy().Head().Data = %q, want %q", copied.Head().Data, list2.Head().Data)
		}
		if copied.Tail() == list2.Tail() {
			t.Error("Copy().Tail() is the same item as Tail()")
		}
		if copied.Tail().Data != list2.Tail().Data {
			t.Errorf("Copy().Tail().Data = %q, want %q", copied.Tail().Data, list2.Tail().Data)
		}
	})

	run(t, "#prepend()", func(t *testing.T) {
		const qux = "qux"

		if got, want := toArray(t, empty.Prepend(NewListItem(qux))), []string{qux}; !slices.Equal(got, want) {
			t.Errorf("empty.Prepend = %q, want %q", got, want)
		}
		if got, want := toArray(t, list1.Prepend(NewListItem(qux))), []string{qux, foo}; !slices.Equal(got, want) {
			t.Errorf("list1.Prepend = %q, want %q", got, want)
		}
		if got, want := toArray(t, list2.Prepend(NewListItem(qux))), []string{qux, foo, bar}; !slices.Equal(got, want) {
			t.Errorf("list2.Prepend = %q, want %q", got, want)
		}
		if got := list2.Prepend(NewListItem("")); got != list2 {
			t.Error("Prepend did not return its receiver")
		}
	})

	run(t, "#prependData()", func(t *testing.T) {
		const qux = "qux"

		if got, want := toArray(t, empty.PrependData(qux)), []string{qux}; !slices.Equal(got, want) {
			t.Errorf("empty.PrependData = %q, want %q", got, want)
		}
		if got, want := toArray(t, list1.PrependData(qux)), []string{qux, foo}; !slices.Equal(got, want) {
			t.Errorf("list1.PrependData = %q, want %q", got, want)
		}
		if got, want := toArray(t, list2.PrependData(qux)), []string{qux, foo, bar}; !slices.Equal(got, want) {
			t.Errorf("list2.PrependData = %q, want %q", got, want)
		}
		if got := list2.PrependData(qux); got != list2 {
			t.Error("PrependData did not return its receiver")
		}
	})

	run(t, "#unshift()", func(t *testing.T) {
		const qux = "qux"

		empty.Unshift(qux)
		if got, want := empty.ToArray(), []string{qux}; !slices.Equal(got, want) {
			t.Errorf("empty after Unshift = %q, want %q", got, want)
		}

		list1.Unshift(qux)
		if got, want := list1.ToArray(), []string{qux, foo}; !slices.Equal(got, want) {
			t.Errorf("list1 after Unshift = %q, want %q", got, want)
		}

		list2.Unshift(qux)
		if got, want := list2.ToArray(), []string{qux, foo, bar}; !slices.Equal(got, want) {
			t.Errorf("list2 after Unshift = %q, want %q", got, want)
		}

		// upstream asserts unshift returns undefined; Unshift has no result.
	})

	t.Run("#shift()", func(t *testing.T) {
		run(t, "should remove first item", func(t *testing.T) {
			head := list2.Head()
			tail := list2.Tail()

			if got := list2.Shift(); got != head {
				t.Errorf("Shift() = %p, want head %p", got, head)
			}
			if got := list2.Shift(); got != tail {
				t.Errorf("Shift() = %p, want tail %p", got, tail)
			}
			if list2.Head() != nil {
				t.Errorf("Head() = %p, want nil", list2.Head())
			}
		})

		run(t, "should return an undefined for an empty list", func(t *testing.T) {
			if got := empty.Shift(); got != nil {
				t.Errorf("Shift() = %+v, want nil", got)
			}
		})
	})

	run(t, "#append()", func(t *testing.T) {
		const qux = "qux"

		if got, want := toArray(t, empty.Append(NewListItem(qux))), []string{qux}; !slices.Equal(got, want) {
			t.Errorf("empty.Append = %q, want %q", got, want)
		}
		if got, want := toArray(t, list1.Append(NewListItem(qux))), []string{foo, qux}; !slices.Equal(got, want) {
			t.Errorf("list1.Append = %q, want %q", got, want)
		}
		if got, want := toArray(t, list2.Append(NewListItem(qux))), []string{foo, bar, qux}; !slices.Equal(got, want) {
			t.Errorf("list2.Append = %q, want %q", got, want)
		}
		if got := list2.Append(NewListItem("")); got != list2 {
			t.Error("Append did not return its receiver")
		}
	})

	run(t, "#appendData()", func(t *testing.T) {
		const qux = "qux"

		if got, want := toArray(t, empty.AppendData(qux)), []string{qux}; !slices.Equal(got, want) {
			t.Errorf("empty.AppendData = %q, want %q", got, want)
		}
		if got, want := toArray(t, list1.AppendData(qux)), []string{foo, qux}; !slices.Equal(got, want) {
			t.Errorf("list1.AppendData = %q, want %q", got, want)
		}
		if got, want := toArray(t, list2.AppendData(qux)), []string{foo, bar, qux}; !slices.Equal(got, want) {
			t.Errorf("list2.AppendData = %q, want %q", got, want)
		}
		if got := list2.AppendData(qux); got != list2 {
			t.Error("AppendData did not return its receiver")
		}
	})

	run(t, "#push()", func(t *testing.T) {
		const qux = "qux"

		empty.Push(qux)
		if got, want := empty.ToArray(), []string{qux}; !slices.Equal(got, want) {
			t.Errorf("empty after Push = %q, want %q", got, want)
		}

		list1.Push(qux)
		if got, want := list1.ToArray(), []string{foo, qux}; !slices.Equal(got, want) {
			t.Errorf("list1 after Push = %q, want %q", got, want)
		}

		list2.Push(qux)
		if got, want := list2.ToArray(), []string{foo, bar, qux}; !slices.Equal(got, want) {
			t.Errorf("list2 after Push = %q, want %q", got, want)
		}

		// upstream asserts push returns undefined; Push has no result.
	})

	t.Run("#pop()", func(t *testing.T) {
		run(t, "should remove last item", func(t *testing.T) {
			head := list2.Head()
			tail := list2.Tail()

			if got := list2.Pop(); got != tail {
				t.Errorf("Pop() = %p, want tail %p", got, tail)
			}
			if got := list2.Pop(); got != head {
				t.Errorf("Pop() = %p, want head %p", got, head)
			}
			if list2.Head() != nil {
				t.Errorf("Head() = %p, want nil", list2.Head())
			}
		})

		run(t, "should return an undefined for an empty list", func(t *testing.T) {
			if got := empty.Pop(); got != nil {
				t.Errorf("Pop() = %+v, want nil", got)
			}
		})
	})

	t.Run("#insert()", func(t *testing.T) {
		run(t, "should append when no ref item", func(t *testing.T) {
			const afterTail = "afterTail"
			res := list2.Insert(NewListItem(afterTail), nil)

			if res != list2 {
				t.Error("Insert did not return its receiver")
			}
			if got := list2.Tail().Data; got != afterTail {
				t.Errorf("Tail().Data = %q, want %q", got, afterTail)
			}
		})

		run(t, "should insert before ref item", func(t *testing.T) {
			const beforeHead = "beforeHead"
			res := list2.Insert(NewListItem(beforeHead), list2.Head())

			if res != list2 {
				t.Error("Insert did not return its receiver")
			}
			if got := list2.Head().Data; got != beforeHead {
				t.Errorf("Head().Data = %q, want %q", got, beforeHead)
			}
		})

		run(t, "insert in the middle", func(t *testing.T) {
			const qux = "qux"
			inserted := NewListItem(qux)
			list2head := list2.Head()
			list2tail := list2.Tail()
			res := list2.Insert(inserted, list2.Tail())

			if res != list2 {
				t.Error("Insert did not return its receiver")
			}
			assertItem(t, "list2head", list2head, nil, inserted, foo)
			assertItem(t, "inserted", inserted, list2head, list2tail, qux)
			assertItem(t, "list2tail", list2tail, inserted, nil, bar)
		})

		run(t, "insert the item before an item that doesn't belong to list", func(t *testing.T) {
			inserted := NewListItem("")

			assertPanicsWith(t, errBeforeNotInList, func() {
				list2.Insert(inserted, list1.Head())
			})
		})
	})

	t.Run("#insertData()", func(t *testing.T) {
		run(t, "should append when no ref item", func(t *testing.T) {
			const afterTail = "afterTail"
			res := list2.InsertData(afterTail, nil)

			if res != list2 {
				t.Error("InsertData did not return its receiver")
			}
			if got := list2.Tail().Data; got != afterTail {
				t.Errorf("Tail().Data = %q, want %q", got, afterTail)
			}
		})

		run(t, "should insert before ref item", func(t *testing.T) {
			const beforeHead = "beforeHead"
			res := list2.InsertData(beforeHead, list2.Head())

			if res != list2 {
				t.Error("InsertData did not return its receiver")
			}
			if got := list2.Head().Data; got != beforeHead {
				t.Errorf("Head().Data = %q, want %q", got, beforeHead)
			}
		})

		run(t, "insert in the middle", func(t *testing.T) {
			const qux = "qux"
			list2head := list2.Head()
			list2tail := list2.Tail()
			res := list2.InsertData(qux, list2.Tail())

			if res != list2 {
				t.Error("InsertData did not return its receiver")
			}
			assertItem(t, "list2head", list2head, nil, list2tail.Prev, foo)
			assertItem(t, "list2tail.Prev", list2tail.Prev, list2head, list2tail, qux)
			assertItem(t, "list2tail", list2tail, list2tail.Prev, nil, bar)
		})

		run(t, "insert the item before an item that doesn't belong to list", func(t *testing.T) {
			assertPanicsWith(t, errBeforeNotInList, func() {
				list2.InsertData("", list1.Head())
			})
		})
	})

	t.Run("#remove()", func(t *testing.T) {
		run(t, "clear a list", func(t *testing.T) {
			head := list2.Head()
			tail := list2.Tail()

			list2.Remove(list2.Tail())
			list2.Remove(list2.Head())

			if list2.Head() != nil || list2.Tail() != nil {
				t.Errorf("Head() = %p, Tail() = %p, want nil", list2.Head(), list2.Tail())
			}
			assertItem(t, "head", head, nil, nil, foo)
			assertItem(t, "tail", tail, nil, nil, bar)
		})

		run(t, "clear a list in reverse order", func(t *testing.T) {
			head := list2.Head()
			tail := list2.Tail()

			list2.Remove(list2.Head())
			list2.Remove(list2.Tail())

			if list2.Head() != nil || list2.Tail() != nil {
				t.Errorf("Head() = %p, Tail() = %p, want nil", list2.Head(), list2.Tail())
			}
			assertItem(t, "head", head, nil, nil, foo)
			assertItem(t, "tail", tail, nil, nil, bar)
		})

		run(t, "remove head item that doesn't belong to list", func(t *testing.T) {
			assertPanicsWith(t, errItemNotInList, func() {
				list1.Remove(list2.Head())
			})
		})

		run(t, "remove tail item that doesn't belong to list", func(t *testing.T) {
			assertPanicsWith(t, errItemNotInList, func() {
				list1.Remove(list2.Tail())
			})
		})
	})

	t.Run("#prependList()", func(t *testing.T) {
		run(t, "prepend non-empty list to non-empty list", func(t *testing.T) {
			list1head := list1.Head()
			list2head := list2.Head()
			res := list2.PrependList(list1)

			if res != list2 {
				t.Error("PrependList did not return its receiver")
			}
			if list1.Head() != nil {
				t.Errorf("list1.Head() = %p, want nil", list1.Head())
			}
			if list2.Head() != list1head {
				t.Errorf("list2.Head() = %p, want list1head %p", list2.Head(), list1head)
			}
			assertItem(t, "list1head", list1head, nil, list2head, foo)
			// upstream expects data bar here, which passes only because foo
			// and bar are deep-equal empty objects; list2head holds foo.
			assertItem(t, "list2head", list2head, list1head, list2.Tail(), foo)
		})

		run(t, "prepend non-empty list to empty list", func(t *testing.T) {
			list2head := list2.Head()
			list2tail := list2.Tail()
			res := empty.PrependList(list2)

			if res != empty {
				t.Error("PrependList did not return its receiver")
			}
			if empty.Head() != list2head || empty.Tail() != list2tail {
				t.Errorf("Head() = %p, Tail() = %p, want %p, %p", empty.Head(), empty.Tail(), list2head, list2tail)
			}
			if list2.Head() != nil || list2.Tail() != nil {
				t.Errorf("list2 Head() = %p, Tail() = %p, want nil", list2.Head(), list2.Tail())
			}
		})

		run(t, "prepend empty list to non-empty", func(t *testing.T) {
			list2head := list2.Head()
			res := list2.PrependList(empty)

			if res != list2 {
				t.Error("PrependList did not return its receiver")
			}
			if empty.Head() != nil {
				t.Errorf("empty.Head() = %p, want nil", empty.Head())
			}
			if list2.Head() != list2head {
				t.Errorf("list2.Head() = %p, want %p", list2.Head(), list2head)
			}
		})
	})

	t.Run("#appendList()", func(t *testing.T) {
		run(t, "append non-empty list to non-empty list", func(t *testing.T) {
			list1head := list1.Head()
			list2tail := list2.Tail()
			res := list2.AppendList(list1)

			if res != list2 {
				t.Error("AppendList did not return its receiver")
			}
			if list1.Head() != nil {
				t.Errorf("list1.Head() = %p, want nil", list1.Head())
			}
			if list2.Tail() != list1head {
				t.Errorf("list2.Tail() = %p, want list1head %p", list2.Tail(), list1head)
			}
			assertItem(t, "list1head", list1head, list2tail, nil, foo)
			assertItem(t, "list2tail", list2tail, list2.Head(), list1head, bar)
		})

		run(t, "append non-empty list to empty list", func(t *testing.T) {
			list2head := list2.Head()
			list2tail := list2.Tail()
			res := empty.AppendList(list2)

			if res != empty {
				t.Error("AppendList did not return its receiver")
			}
			if empty.Head() != list2head || empty.Tail() != list2tail {
				t.Errorf("Head() = %p, Tail() = %p, want %p, %p", empty.Head(), empty.Tail(), list2head, list2tail)
			}
			if list2.Head() != nil || list2.Tail() != nil {
				t.Errorf("list2 Head() = %p, Tail() = %p, want nil", list2.Head(), list2.Tail())
			}
		})

		run(t, "append empty list to non-empty", func(t *testing.T) {
			list2tail := list2.Tail()
			res := list2.AppendList(empty)

			if res != list2 {
				t.Error("AppendList did not return its receiver")
			}
			if empty.Head() != nil {
				t.Errorf("empty.Head() = %p, want nil", empty.Head())
			}
			if list2.Tail() != list2tail {
				t.Errorf("list2.Tail() = %p, want %p", list2.Tail(), list2tail)
			}
		})
	})

	t.Run("#insertList()", func(t *testing.T) {
		run(t, "add non-empty list to non-empty list", func(t *testing.T) {
			list1head := list1.Head()
			res := list2.InsertList(list1, list2.Tail())

			if res != list2 {
				t.Error("InsertList did not return its receiver")
			}
			if list1.Head() != nil || list1.Tail() != nil {
				t.Errorf("list1 Head() = %p, Tail() = %p, want nil", list1.Head(), list1.Tail())
			}
			assertItem(t, "list1head", list1head, list2.Head(), list2.Tail(), foo)
			// upstream expects data bar here, which passes only because foo
			// and bar are deep-equal empty objects; list2.head holds foo.
			assertItem(t, "list2.Head()", list2.Head(), nil, list1head, foo)
			assertItem(t, "list2.Tail()", list2.Tail(), list1head, nil, bar)
		})

		run(t, "add non-empty list to empty list", func(t *testing.T) {
			list2head := list2.Head()
			list2tail := list2.Tail()
			res := empty.InsertList(list2, nil)

			if res != empty {
				t.Error("InsertList did not return its receiver")
			}
			if empty.Head() != list2head || empty.Tail() != list2tail {
				t.Errorf("Head() = %p, Tail() = %p, want %p, %p", empty.Head(), empty.Tail(), list2head, list2tail)
			}
			if list2.Head() != nil || list2.Tail() != nil {
				t.Errorf("list2 Head() = %p, Tail() = %p, want nil", list2.Head(), list2.Tail())
			}
		})

		run(t, "add empty list to non-empty", func(t *testing.T) {
			list2tail := list2.Tail()
			res := list2.InsertList(empty, nil)

			if res != list2 {
				t.Error("InsertList did not return its receiver")
			}
			if empty.Head() != nil {
				t.Errorf("empty.Head() = %p, want nil", empty.Head())
			}
			if list2.Tail() != list2tail {
				t.Errorf("list2.Tail() = %p, want %p", list2.Tail(), list2tail)
			}
		})
	})

	t.Run("#replace()", func(t *testing.T) {
		run(t, "replace for an item", func(t *testing.T) {
			const qux = "qux"
			list2.Replace(list2.Tail(), NewListItem(qux))

			if got, want := toArray(t, list2), []string{foo, qux}; !slices.Equal(got, want) {
				t.Errorf("list2 after Replace = %q, want %q", got, want)
			}
		})

		run(t, "replace for a list", func(t *testing.T) {
			list2.ReplaceList(list2.Tail(), list1)

			if got, want := toArray(t, list2), []string{foo, foo}; !slices.Equal(got, want) {
				t.Errorf("list2 after ReplaceList = %q, want %q", got, want)
			}
		})
	})
}

// Not in upstream: upstream only mutates around the current item, which
// never needs a cursor update. Removing or inserting an item the iteration
// has not reached yet must be seen by the iteration, which only the cursor
// updates guarantee.
func TestListMutateAhead(t *testing.T) {
	type visitFunc = func(data string, item *ListItem[string], list *List[string])

	forward := map[string]func(list *List[string], fn visitFunc){
		"forEach": func(list *List[string], fn visitFunc) { list.ForEach(fn) },
		"reduce": func(list *List[string], fn visitFunc) {
			Reduce(list, func(acc int, data string, item *ListItem[string], list *List[string]) int {
				fn(data, item, list)
				return acc
			}, 0)
		},
		"nextUntil": func(list *List[string], fn visitFunc) {
			list.NextUntil(list.Head(), func(data string, item *ListItem[string], list *List[string]) bool {
				fn(data, item, list)
				return false
			})
		},
	}
	backward := map[string]func(list *List[string], fn visitFunc){
		"forEachRight": func(list *List[string], fn visitFunc) { list.ForEachRight(fn) },
		"reduceRight": func(list *List[string], fn visitFunc) {
			ReduceRight(list, func(acc int, data string, item *ListItem[string], list *List[string]) int {
				fn(data, item, list)
				return acc
			}, 0)
		},
		"prevUntil": func(list *List[string], fn visitFunc) {
			list.PrevUntil(list.Tail(), func(data string, item *ListItem[string], list *List[string]) bool {
				fn(data, item, list)
				return false
			})
		},
	}

	for _, tt := range []struct {
		name    string
		methods map[string]func(list *List[string], fn visitFunc)
		mutate  func(data string, item *ListItem[string], list *List[string])
		want    []string
	}{
		{
			name:    "remove next",
			methods: forward,
			mutate: func(data string, item *ListItem[string], list *List[string]) {
				if data == "a" {
					list.Remove(item.Next)
				}
			},
			want: []string{"a", "c"},
		},
		{
			name:    "insert before next",
			methods: forward,
			mutate: func(data string, item *ListItem[string], list *List[string]) {
				if data == "a" {
					list.InsertData("x", item.Next)
				}
			},
			want: []string{"a", "x", "b", "c"},
		},
		{
			name:    "remove prev",
			methods: backward,
			mutate: func(data string, item *ListItem[string], list *List[string]) {
				if data == "c" {
					list.Remove(item.Prev)
				}
			},
			want: []string{"c", "a"},
		},
		{
			name:    "insert before current",
			methods: backward,
			mutate: func(data string, item *ListItem[string], list *List[string]) {
				if data == "c" {
					list.InsertData("x", item)
				}
			},
			want: []string{"c", "x", "b", "a"},
		},
	} {
		for method, iterate := range tt.methods {
			t.Run(method+"/"+tt.name, func(t *testing.T) {
				list := new(List[string]).FromArray([]string{"a", "b", "c"})
				var visited []string

				iterate(list, func(data string, item *ListItem[string], list *List[string]) {
					visited = append(visited, data)
					tt.mutate(data, item, list)
				})

				if !slices.Equal(visited, tt.want) {
					t.Errorf("%s visited %q, want %q", method, visited, tt.want)
				}
				if list.cursor != nil {
					t.Errorf("%s left a cursor on the list", method)
				}
			})
		}
	}
}
