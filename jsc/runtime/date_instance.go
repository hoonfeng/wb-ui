// DateInstance corresponds to JSC::DateInstance (runtime/DateInstance.h)
package runtime

import (
	"math"
	"time"
)

// DateInstance corresponds to JSC::DateInstance.
type DateInstance struct {
	JSNonFinalObject
	internalNumber float64 // milliseconds since epoch (NaN = invalid date)
}

// NewDateInstance creates a new DateInstance.
func NewDateInstance(vm *VM, structure *Structure, date float64) *DateInstance {
	d := &DateInstance{
		internalNumber: date,
	}
	d.structureID = structure.structureID
	d.typ = ObjectType
	d.cellState = DefinitelyWhite
	d.properties = make(map[string]JSValue)
	_ = vm
	return d
}

// InternalValue returns the internal time value.
func (d *DateInstance) InternalValue() float64 { return d.internalNumber }

// SetInternalValue sets the internal time value.
func (d *DateInstance) SetInternalValue(v float64) { d.internalNumber = v }

// ToDateString returns a human-readable date string.
func (d *DateInstance) ToDateString() string {
	if math.IsNaN(d.internalNumber) {
		return "Invalid Date"
	}
	t := time.UnixMilli(int64(d.internalNumber))
	return t.Format("Mon Jan 02 2006")
}

// ToISOString returns an ISO 8601 date string.
func (d *DateInstance) ToISOString() string {
	if math.IsNaN(d.internalNumber) {
		return "Invalid Date"
	}
	t := time.UnixMilli(int64(d.internalNumber))
	return t.Format("2006-01-02T15:04:05.000Z")
}
