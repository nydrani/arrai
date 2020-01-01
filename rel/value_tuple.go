package rel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/marcelocantos/frozen"
)

// GenericTuple is the default implementation of Tuple.
type GenericTuple struct {
	tuple frozen.Map
	names []string
}

var (
	// EmptyTuple is the tuple with no attributes.
	EmptyTuple Tuple = &GenericTuple{}

	negateTag = "@neg"
)

// NewAttr returns an Attr with the given name and value.
func NewAttr(name string, value Value) Attr {
	return Attr{Name: name, Value: value}
}

// NewBoolAttr return an attr with a bool value.
func NewBoolAttr(name string, value bool) Attr {
	return NewAttr(name, NewBool(value))
}

// NewFloatAttr return an attr with a float value.
func NewFloatAttr(name string, value float64) Attr {
	return NewAttr(name, NewNumber(value))
}

// NewIntAttr return an attr with an int value.
func NewIntAttr(name string, value int) Attr {
	return NewFloatAttr(name, float64(value))
}

// NewStringAttr return an attr with a string value.
func NewStringAttr(name string, value []rune) Attr {
	return NewAttr(name, NewString(value))
}

// NewTupleAttr return an attr with a new tuple value.
func NewTupleAttr(name string, attrs ...Attr) Attr {
	return NewAttr(name, NewTuple(attrs...))
}

// NewTuple constructs a Tuple from attrs. Passes each Val to NewValue().
func NewTuple(attrs ...Attr) Tuple {
	var b frozen.MapBuilder
	for _, kv := range attrs {
		b.Put(kv.Name, kv.Value)
	}
	return &GenericTuple{tuple: b.Finish()}
}

// NewTupleFromMap constructs a Tuple from a map of strings to Go values.
func NewTupleFromMap(m map[string]interface{}) (Tuple, error) {
	var b frozen.MapBuilder
	for name, intf := range m {
		value, err := NewValue(intf)
		if err != nil {
			return nil, err
		}
		b.Put(name, value)
	}
	return &GenericTuple{tuple: b.Finish()}, nil
}

// NewXML constructs an XML Tuple from the given data
func NewXML(tag []rune, attrs []Attr, children ...Value) Tuple {
	var b frozen.MapBuilder
	b.Put("tag", NewString(tag))
	if len(attrs) != 0 {
		b.Put("attributes", NewTuple(attrs...))
	}
	if len(children) != 0 {
		b.Put("children", NewArray(children...))
	}
	return EmptyTuple.With("@xml", &GenericTuple{tuple: b.Finish()})
}

// Hash computes a hash for a GenericTuple.
func (t *GenericTuple) Hash(seed uintptr) uintptr {
	return t.tuple.Hash(seed)
}

// Equal tests two Tuples for equality. Any other type returns false.
func (t *GenericTuple) Equal(v interface{}) bool {
	if b, ok := v.(Tuple); ok {
		for e := t.Enumerator(); e.MoveNext(); {
			aName, aValue := e.Current()
			if bVal, found := b.Get(aName); found {
				if !aValue.Equal(bVal) {
					return false
				}
			} else {
				return false
			}
		}
		for e := b.Enumerator(); e.MoveNext(); {
			name, _ := e.Current()
			if _, found := t.Get(name); !found {
				return false
			}
		}
		return true
	}
	return false
}

// LexerNamePat defines valid unquoted identifiers.
// This really belongs in rel/syntax/lex.go, but that creates a dep cycle.
var LexerNamePat = `([$@A-Za-z_][0-9$@A-Za-z_]*)`

var identRE = regexp.MustCompile(`\A` + LexerNamePat + `\z`)

// String returns a string representation of a Tuple.
func (t *GenericTuple) String() string {
	var buf bytes.Buffer
	buf.WriteRune('(')
	for i, name := range tupleOrderedNames(t) {
		if i != 0 {
			buf.WriteString(", ")
		}
		if identRE.Match([]byte(name)) {
			buf.WriteString(name)
		} else {
			data, err := json.Marshal(name)
			if err != nil {
				panic(err)
			}
			buf.Write(data)
		}
		buf.WriteString(": ")
		value, found := t.Get(name)
		if !found {
			panic(fmt.Sprintf(
				"walk() produced name, %v, which fails lookup", name))
		}
		buf.WriteString(value.String())
	}
	buf.WriteRune(')')
	return buf.String()
}

// Eval returns the tuple.
func (t *GenericTuple) Eval(local, global *Scope) (Value, error) {
	return t, nil
}

// Kind returns a number that is unique for each major kind of Value.
func (t *GenericTuple) Kind() int {
	if t.Count() == 1 {
		if x, ok := t.Get(negateTag); ok {
			return -x.Kind()
		}
	}
	return 300
}

// Bool returns true iff the tuple has attributes.
func (t *GenericTuple) Bool() bool {
	return t.Count() > 0
}

// Less returns true iff v is not a number or tuple, or v is a tuple and t
// precedes v in a lexicographical comparison of their name/value pairs.
func (t *GenericTuple) Less(v Value) bool {
	if t.Kind() != v.Kind() {
		return t.Kind() < v.Kind()
	}
	if t.Count() == 1 {
		if x, ok := t.Get(negateTag); ok {
			u := v.(Tuple)
			if u.Count() != 1 {
				panic(negateTag + " kind not single-attr tuple")
			}
			if y, ok := v.(Tuple).Get(negateTag); ok {
				return y.Less(x)
			}
			panic(negateTag + " kind missing " + negateTag + " attr")
		}
	}

	x := v.(*GenericTuple)
	a := tupleOrderedNames(t)
	b := tupleOrderedNames(x)
	n := len(a)
	if n > len(b) {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
		va, _ := t.Get(a[i])
		vb, _ := x.Get(b[i])
		if va.Less(vb) {
			return true
		}
		if vb.Less(va) {
			return false
		}
	}
	return len(a) < len(b)
}

// Negate returns x if t matches {(negateTag): x} else {(negateTag): t}.
func (t *GenericTuple) Negate() Value {
	if t.Count() == 1 {
		if x, ok := t.Get(negateTag); ok {
			return x
		}
	}
	if !t.Bool() {
		return t
	}
	return NewTuple(NewAttr(negateTag, t))
}

// Export exports a Tuple.
func (t *GenericTuple) Export() interface{} {
	result := make(map[string]interface{}, t.Count())
	for e := t.Enumerator(); e.MoveNext(); {
		name, value := e.Current()
		result[name] = value.Export()
	}
	return result
}

// Count returns how many attributes are in the Tuple.
func (t *GenericTuple) Count() int {
	return t.tuple.Count()
}

// Get returns the Value associated with a name, and true iff it was found.
func (t *GenericTuple) Get(name string) (Value, bool) {
	if v, found := t.tuple.Get(name); found {
		return v.(Value), true
	}
	return nil, false
}

// With returns a Tuple with all name/Value pairs in t (except the one for the
// given name, if present) with the addition of the given name/Value pair.
func (t *GenericTuple) With(name string, value Value) Tuple {
	// Strip view/non-view counterpart.
	if strings.HasPrefix(name, "&") {
		t = t.Without(name[1:]).(*GenericTuple)
	} else {
		t = t.Without("&" + name).(*GenericTuple)
	}
	return &GenericTuple{tuple: t.tuple.With(name, value)}
}

// Without returns a Tuple with all name/Value pairs in t exception the one of
// the given name.
func (t *GenericTuple) Without(name string) Tuple {
	return &GenericTuple{tuple: t.tuple.Without(frozen.NewSet(name))}
}

// HasName returns true iff the Tuple has an attribute with the given name.
func (t *GenericTuple) HasName(name string) bool {
	_, found := t.tuple.Get(name)
	return found
}

// Attributes returns attributes as a map.
func (t *GenericTuple) Attributes() map[string]Value {
	attrs := make(map[string]Value, t.Count())
	for e := t.Enumerator(); e.MoveNext(); {
		name, value := e.Current()
		attrs[name] = value
	}
	return attrs
}

// Names returns the attribute names.
func (t *GenericTuple) Names() Names {
	var b frozen.SetBuilder
	for e := t.Enumerator(); e.MoveNext(); {
		name, _ := e.Current()
		b.Add(name)
	}
	return Names(b.Finish())
}

// Project returns a tuple with the given names from this tuple, or nil if any
// name wasn't found.
func (t *GenericTuple) Project(names Names) Tuple {
	var b frozen.MapBuilder
	for e := names.Enumerator(); e.MoveNext(); {
		name := e.Current()
		value, found := t.Get(name)
		if !found {
			return nil
		}
		b.Put(name, value)
	}
	return &GenericTuple{tuple: b.Finish()}
}

// GenericTupleEnumerator represents an enumerator over a GenericTuple.
type GenericTupleEnumerator struct {
	i *frozen.MapIterator
}

// MoveNext moves the enumerator to the next Value.
func (e *GenericTupleEnumerator) MoveNext() bool {
	return e.i.Next()
}

// Current returns the enumerator's current Value.
func (e *GenericTupleEnumerator) Current() (string, Value) {
	k, v := e.i.Entry()
	return k.(string), v.(Value)
}

// Enumerator returns an enumerator over the Values in the GenericTuple.
func (t *GenericTuple) Enumerator() AttrEnumerator {
	return &GenericTupleEnumerator{i: t.tuple.Range()}
}

// orderedNames returns the names of this tuple in sorted order.
func tupleOrderedNames(t *GenericTuple) []string {
	if len(t.names) == 0 {
		for e := t.Enumerator(); e.MoveNext(); {
			name, _ := e.Current()
			t.names = append(t.names, name)
		}
		sort.Strings(t.names)
	}
	return t.names
}
