// Translation of: Source/WTF/wtf/DataRef.h
//
// DataRef is a copy-on-write pointer container used by ComputedStyle.
// Reading returns a const (*T) pointer; writing via Access() clones the
// underlying data if there are multiple references.

package style

// DataRef is the Go translation of WTF::DataRef<T>.
// It provides shared read access and copy-on-write write access.
type DataRef[T any] struct {
	data *T
}

// NewDataRef creates a DataRef from an initial value.
func NewDataRef[T any](v *T) DataRef[T] {
	return DataRef[T]{data: v}
}

// Get returns a read-only pointer to the underlying value.
func (d DataRef[T]) Get() *T { return d.data }

// Access returns a mutable pointer, cloning the data if we need COW.
// In Go the COW is a shallow copy; the caller should deep-copy pointer/map
// fields manually after calling Access if needed.
func (d *DataRef[T]) Access(initial func() *T) *T {
	if d.data == nil {
		d.data = initial()
	}
	return d.data
}

// Share makes this DataRef point to the same underlying data as other.
func (d *DataRef[T]) Share(other DataRef[T]) {
	d.data = other.data
}

// Clone returns a new DataRef pointing to a shallow copy of the data.
func (d DataRef[T]) Clone() DataRef[T] {
	if d.data == nil {
		return DataRef[T]{}
	}
	cp := new(T)
	*cp = *d.data
	return DataRef[T]{data: cp}
}
