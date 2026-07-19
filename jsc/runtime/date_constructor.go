// Translation of: Source/JavaScriptCore/runtime/DateConstructor.h
//                  Source/JavaScriptCore/runtime/DateConstructor.cpp
//
// DateConstructor implements the Date() constructor.
// ES 21.4.2 The Date Constructor

package runtime

import (
	"fmt"
	"math"
	"time"
)

// Date time constants.
const (
	msPerSecond = 1000
	msPerMinute = 60000
	msPerHour   = 3600000
	msPerDay    = 86400000
)

// TimeType corresponds to JSC::TimeType.
type TimeType uint8

const (
	TimeTypeLocalTime TimeType = iota
	TimeTypeUTCTime
)

// DateConstructor corresponds to JSC::DateConstructor.
type DateConstructor struct {
	InternalFunction
}

// DateConstructorStructureFlags are the StructureFlags for DateConstructor.
const DateConstructorStructureFlags uint32 = InternalFunctionStructureFlags | HasStaticPropertyTable

// NewDateConstructor creates a new DateConstructor.
// In C++: static DateConstructor* create(VM&, Structure*, DatePrototype*)
func NewDateConstructor(vm *VM, structure *Structure, datePrototype *DatePrototype) *DateConstructor {
	c := &DateConstructor{}
	c.InternalFunction = InternalFunction{
		JSNonFinalObject: JSNonFinalObject{},
		functionForCall:      callDate,
		functionForConstruct: constructWithDateConstructor,
	}
	c.structureID = structure.structureID
	c.typ = InternalFunctionType
	c.cellState = DefinitelyWhite
	c.properties = make(map[string]JSValue)
	c.FinishCreation(vm, datePrototype)
	return c
}

// FinishCreation completes DateConstructor initialization.
// In C++: void finishCreation(VM&, DatePrototype*)
func (c *DateConstructor) FinishCreation(vm *VM, datePrototype *DatePrototype) {
	// Base::finishCreation(vm, 7, "Date", WithoutStructureTransition)
	c.InternalFunction.FinishCreation(vm, 7, "Date")

	// putDirectWithoutTransition(vm, vm.propertyNames->prototype, datePrototype, DontEnum|DontDelete|ReadOnly)
	c.putDirectWithoutTransition(vm, NewPropertyName("prototype"),
		NewJSValueObject(&datePrototype.JSObject),
		PropertyAttributeDontEnum|PropertyAttributeDontDelete|PropertyAttributeReadOnly)

	// Static methods: Date.now(), Date.parse(), Date.UTC()
	c.putDirectWithoutTransition(vm, NewPropertyName("now"),
		NewJSValueObject(nil), // placeholder
		PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("parse"),
		NewJSValueObject(nil), // placeholder
		PropertyAttributeDontEnum)
	c.putDirectWithoutTransition(vm, NewPropertyName("UTC"),
		NewJSValueObject(nil), // placeholder
		PropertyAttributeDontEnum)

	_ = vm
}

// ===== Call / Construct =====

// callDate implements [[Call]] for the Date constructor.
// ES 21.4.2.1 Date ( ...values ) — called as a function.
// Returns the current date/time as a string.
// In C++: JSC_DEFINE_HOST_FUNCTION(callDate)
func callDate(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	// Return the current date/time as a string (like new Date().toString())
	now := jsCurrentTime()
	_ = callFrame

	// Simplified: format as ISO-like date string
	t := time.UnixMilli(int64(now))
	return JSValueEncode(NewJSValueString(t.Format("Mon Jan 2 2006 15:04:05 GMT-0700 (MST)")))
}

// constructWithDateConstructor implements [[Construct]] for the Date constructor.
// ES 21.4.2.1 Date ( ...values ) — called with new.
// In C++: JSC_DEFINE_HOST_FUNCTION(constructWithDateConstructor)
func constructWithDateConstructor(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	args := callFrame.Arguments()
	result := constructDate(globalObject, callFrame.NewTarget(), args)
	if result == nil {
		return EncodedJSValue()
	}
	return JSValueEncode(NewJSValueObject(result))
}

// ===== Main construction logic =====

// constructDate is the core Date construction logic.
// ES 21.4.2.1 Date ( ...values )
// In C++: JSObject* constructDate(JSGlobalObject*, JSValue newTarget, const ArgList&)
func constructDate(globalObject *JSGlobalObject, newTarget JSValue, args []JSValue) *JSObject {
	vm := globalObject.VM()
	numArgs := len(args)

	var value float64

	if numArgs == 0 {
		// new Date() — current time
		value = jsCurrentTime()
	} else if numArgs == 1 {
		arg0 := args[0]
		// Check if arg0 is a DateInstance (type == JSDateType)
		if arg0.IsObject() && arg0.GetObject().typ == JSDateType {
			// Convert Date to its internal time value via ToPrimitive
			primitive := arg0.ToPrimitive(NoPreference)
			if primitive.IsString() {
				value = parseDateString(primitive.ToString())
			} else {
				value = primitive.ToNumber()
			}
		} else {
// Try toPrimitive
			primitive := arg0.ToPrimitive(NoPreference)
			
			if primitive.IsString() {
				// Parse date string
				value = parseDateString(primitive.ToString())
				if math.IsNaN(value) {
					value = math.NaN()
				}
			} else {
				value = primitive.ToNumber()
				if math.IsNaN(value) {
					value = math.NaN()
				}
			}
		}
	} else {
		// Multiple arguments: year, month, day, hours, minutes, seconds, ms
		value = millisecondsFromComponents(globalObject, args, TimeTypeLocalTime)
	}

	// Determine structure
	var dateStructure *Structure
	if newTarget.IsUndefined() {
		typeInfo := NewTypeInfo(ObjectType, 0)
		dateStructure = NewStructure(vm, globalObject, JSValueNull, typeInfo, &ClassInfo{})
	} else {
		typeInfo := NewTypeInfo(ObjectType, 0)
		dateStructure = NewStructure(vm, globalObject, JSValueNull, typeInfo, &ClassInfo{})
		_ = newTarget
	}

	return NewJSValueObject(&NewDateInstance(vm, dateStructure, value).JSNonFinalObject.JSObject).GetObject()
}

// ===== Static method implementations =====

// dateNow implements Date.now()
// ES 21.4.3.1 Date.now()
// In C++: JSC_DEFINE_HOST_FUNCTION(dateNow)
func dateNow(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	_ = globalObject
	_ = callFrame
	ms := jsCurrentTime()
	return JSValueEncode(jsNumber(ms))
}

// dateParse implements Date.parse(string)
// ES 21.4.3.2 Date.parse ( string )
// In C++: JSC_DEFINE_HOST_FUNCTION(dateParse)
func dateParse(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	dateStr := callFrame.Argument(0).ToString()
	ms := parseDateString(dateStr)
	if math.IsNaN(ms) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	clipped := timeClip(ms)
	return JSValueEncode(jsNumber(clipped))
}

// dateUTC implements Date.UTC(year, month, day, hours, minutes, seconds, ms)
// ES 21.4.3.4 Date.UTC ( year, month [, date [, hours [, minutes [, seconds [, ms ] ] ] ] ] )
// In C++: JSC_DEFINE_HOST_FUNCTION(dateUTC)
func dateUTC(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	args := callFrame.Arguments()
	ms := millisecondsFromComponents(globalObject, args, TimeTypeUTCTime)
	return JSValueEncode(jsNumber(ms))
}

// ===== Helper functions =====

// jsCurrentTime returns the current time in milliseconds since epoch.
// In C++: jsCurrentTime() → WallTime::now().secondsSinceEpoch().milliseconds()
func jsCurrentTime() float64 {
	return float64(time.Now().UnixMilli())
}

// millisecondsFromComponents computes milliseconds from date components.
// In C++: static double millisecondsFromComponents(JSGlobalObject*, const ArgList&, TimeType)
func millisecondsFromComponents(globalObject *JSGlobalObject, args []JSValue, timeType TimeType) float64 {
	vm := globalObject.VM()
	_ = vm

	// Initialize with default values: year, month, day, hours, minutes, seconds, ms
	doubleArgs := [7]float64{0, 0, 1, 0, 0, 0, 0}
	hasNonFinite := false

	numUsed := len(args)
	if numUsed > 7 {
		numUsed = 7
	}
	if numUsed < 1 {
		numUsed = 1
	}

	for i := 0; i < numUsed; i++ {
		doubleArgs[i] = args[i].ToNumber()
		if math.IsNaN(doubleArgs[i]) || math.IsInf(doubleArgs[i], 0) {
			hasNonFinite = true
		}
		doubleArgs[i] = math.Trunc(doubleArgs[i])
	}

	if hasNonFinite {
		return math.NaN()
	}

	// Year 0-99 → add 1900
	if doubleArgs[0] >= 0 && doubleArgs[0] <= 99 {
		doubleArgs[0] += 1900
	}

	time := makeDate(makeDay(doubleArgs[0], doubleArgs[1], doubleArgs[2]),
		makeTime(doubleArgs[3], doubleArgs[4], doubleArgs[5], doubleArgs[6]))

	// Simplified: no timezone adjustment for now
	_ = timeType
	return timeClip(time)
}

// makeDay implements ES 21.4.1.2 MakeDay ( year, month, date )
// In C++: constexpr static inline double makeDay(double year, double month, double date)
func makeDay(year, month, date float64) float64 {
	additionalYears := math.Floor(month / 12)
	ym := year + additionalYears
	if math.IsInf(ym, 0) || math.IsNaN(ym) {
		return math.NaN()
	}
	mm := month - additionalYears*12
	yearInt32 := int32(ym)
	monthInt32 := int32(mm)
	if float64(yearInt32) != ym || float64(monthInt32) != mm {
		return math.NaN()
	}
	days := dateToDaysFrom1970(yearInt32, monthInt32, 1)
	return days + date - 1
}

// makeDate implements ES 21.4.1.1 MakeDate ( day, time )
// In C++: constexpr static inline double makeDate(double day, double time)
func makeDate(day, time float64) float64 {
	return day*msPerDay + time
}

// makeTime implements ES 21.4.1.3 MakeTime ( hour, min, sec, ms )
// In C++: constexpr static inline double makeTime(double hour, double min, double sec, double ms)
func makeTime(hour, min, sec, ms float64) float64 {
	return (hour*msPerHour + min*msPerMinute) + sec*msPerSecond + ms
}

// timeClip implements ES 21.4.1.15 TimeClip ( time )
// Clips time to valid range: [-8.64e15, 8.64e15] (ES spec ±8640000000000000)
func timeClip(time float64) float64 {
	if math.IsNaN(time) || math.IsInf(time, 0) || math.Abs(time) > 8.64e15 {
		return math.NaN()
	}
	return math.Trunc(time)
}

// parseDateString parses a date string and returns milliseconds since epoch.
// Simplified: uses Go time.Parse with common formats.
// In C++ this uses vm.dateCache.parseDate() which does full ES parsing.
func parseDateString(s string) float64 {
	// Try ISO 8601 format: "2006-01-02T15:04:05Z07:00"
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02",
		time.RFC1123Z,
		time.RFC1123,
		time.ANSIC,
		time.UnixDate,
		time.RubyDate,
		"Mon, 02 Jan 2006 15:04:05 -0700",
		"2 Jan 2006 15:04:05",
		"Jan 2, 2006",
		"January 2, 2006",
		"01/02/2006",
		"2006/01/02",
	}

	// Try parsing with layout
	for _, layout := range formats {
		if t, err := time.Parse(layout, s); err == nil {
			return float64(t.UnixMilli())
		}
	}

	// Try as a timestamp number
	if t, err := parseFloat(s); err == nil {
		return t
	}

	return math.NaN()
}

// parseFloat is a simplified float64 string parser.
func parseFloat(s string) (float64, error) {
	// Try converting the entire string
	var val float64
	_, err := fmt.Sscanf(s, "%f", &val)
	return val, err
}

// dateToDaysFrom1970 computes days since 1970-01-01 for a given year/month/day.
// Simplified implementation using Go time.
func dateToDaysFrom1970(year int32, month int32, day int32) float64 {
	t := time.Date(int(year), time.Month(month+1), int(day), 0, 0, 0, 0, time.UTC) // month is 0-indexed
	return float64(t.UnixMilli() / msPerDay)
}
