// Translation of: Source/JavaScriptCore/runtime/DatePrototype.h
//                  Source/JavaScriptCore/runtime/DatePrototype.cpp
//
// DatePrototype is the prototype for all JavaScript Date objects (Date.prototype).
// ES 21.4.4 Properties of the Date Prototype Object

package runtime

import (
	"fmt"
	"math"
	"time"
)

// DatePrototype corresponds to JSC::DatePrototype.
type DatePrototype struct {
	JSNonFinalObject
}

// NewDatePrototype creates a new DatePrototype.
// In C++: static DatePrototype* create(VM&, JSGlobalObject*, Structure*)
func NewDatePrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *DatePrototype {
	p := &DatePrototype{}
	p.structureID = structure.structureID
	p.typ = JSDateType
	p.cellState = DefinitelyWhite
	p.properties = make(map[string]JSValue)
	p.FinishCreation(vm, globalObject)
	return p
}

// FinishCreation registers all Date.prototype methods.
// In C++: void finishCreation(VM&, JSGlobalObject*)
func (p *DatePrototype) FinishCreation(vm *VM, globalObject *JSGlobalObject) {
	p.JSNonFinalObject.FinishCreation(vm)

	// Getter methods (local time)
	p.putDirectWithoutTransition(vm, NewPropertyName("getDate"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("getDay"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("getFullYear"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("getHours"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("getMilliseconds"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("getMinutes"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("getMonth"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("getSeconds"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("getTime"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("getTimezoneOffset"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("getYear"), NewJSValueObject(nil), PropertyAttributeDontEnum)

	// Getter methods (UTC)
	p.putDirectWithoutTransition(vm, NewPropertyName("getUTCDate"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("getUTCDay"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("getUTCFullYear"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("getUTCHours"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("getUTCMilliseconds"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("getUTCMinutes"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("getUTCMonth"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("getUTCSeconds"), NewJSValueObject(nil), PropertyAttributeDontEnum)

	// Setter methods (local time)
	p.putDirectWithoutTransition(vm, NewPropertyName("setDate"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("setFullYear"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("setHours"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("setMilliseconds"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("setMinutes"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("setMonth"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("setSeconds"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("setTime"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("setYear"), NewJSValueObject(nil), PropertyAttributeDontEnum)

	// Setter methods (UTC)
	p.putDirectWithoutTransition(vm, NewPropertyName("setUTCDate"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("setUTCFullYear"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("setUTCHours"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("setUTCMilliseconds"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("setUTCMinutes"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("setUTCMonth"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("setUTCSeconds"), NewJSValueObject(nil), PropertyAttributeDontEnum)

	// String conversion methods
	p.putDirectWithoutTransition(vm, NewPropertyName("toString"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("toDateString"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("toTimeString"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("toUTCString"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("toISOString"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("toJSON"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("toLocaleString"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("toLocaleDateString"), NewJSValueObject(nil), PropertyAttributeDontEnum)
	p.putDirectWithoutTransition(vm, NewPropertyName("toLocaleTimeString"), NewJSValueObject(nil), PropertyAttributeDontEnum)

	// Value access
	p.putDirectWithoutTransition(vm, NewPropertyName("valueOf"), NewJSValueObject(nil), PropertyAttributeDontEnum)

	// @@toPrimitive
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolToPrimitive), NewJSValueObject(nil), PropertyAttributeDontEnum)

	// @@toStringTag
	p.putDirectWithoutTransition(vm, NewPropertyName(SymbolToStringTag), NewJSValueString("Date"), PropertyAttributeDontEnum)

	_ = globalObject
}

// =====================================================================
// Helpers
// =====================================================================

// thisTimeValue extracts the internal time value from a Date-like object.
// ES 21.4.4.3 thisTimeValue ( value )
// Returns the internal millisecond timestamp, or NaN if not a valid Date.
func thisTimeValue(globalObject *JSGlobalObject, thisValue JSValue) float64 {
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return math.NaN()
	}
	// Check if it's a DateInstance (has internalNumber)
	if di := dynamicDowncast[DateInstance](thisObj); di != nil {
		return di.InternalValue()
	}
	// Fallback: look for internalNumber on the object (simplified)
	if val, ok := getInternalDateValue(thisObj); ok {
		return val
	}
	return math.NaN()
}

// getInternalDateValue is a helper that extracts date value from a generic JSObject.
// In simplified mode, tries to find the "[[DateValue]]" property or similar.
func getInternalDateValue(obj *JSObject) (float64, bool) {
	_ = obj
	// Simplified: return NaN indicating not a valid Date
	return math.NaN(), false
}

// msToTime converts milliseconds since epoch to Go time.Time (local timezone).
func msToTime(ms float64) time.Time {
	if math.IsNaN(ms) {
		return time.Time{}
	}
	return time.UnixMilli(int64(ms))
}

// msToTimeUTC converts to Go time.Time (UTC).
func msToTimeUTC(ms float64) time.Time {
	if math.IsNaN(ms) {
		return time.Time{}
	}
	return time.UnixMilli(int64(ms)).UTC()
}

// timeToMs converts Go time.Time back to milliseconds since epoch.
func timeToMs(t time.Time) float64 {
	return float64(t.UnixMilli())
}

// isValidTime checks if a time value is valid (not NaN, not Inf).
func isValidTime(ms float64) bool {
	return !math.IsNaN(ms) && !math.IsInf(ms, 0)
}

// clampToValidRange ensures time is within ES-specified range (±8.64e15)
func clampToValidRange(ms float64) float64 {
	return timeClip(ms)
}

// dateFromArgs constructs a time from setter arguments.
// pattern specifies which arguments to expect:
//   "y,m,d" for setFullYear, "h,m,s,ms" for setHours, etc.
// Returns the new time value in milliseconds.
func dateFromSetterArgs(ms float64, args []JSValue, pattern string) float64 {
	if !isValidTime(ms) {
		return ms // propagate NaN
	}

	t := msToTime(ms)
	year, month, day := t.Date()
	hour, min, sec := t.Clock()
	nsec := t.Nanosecond()

	// Default all to current values, then override based on pattern
	switch pattern {
	case "day":
		if len(args) > 0 {
			day = int(args[0].ToNumber())
		}
	case "month":
		if len(args) > 0 {
			month = time.Month(int(args[0].ToNumber()) + 1) // JS month 0-based
		}
		if len(args) > 1 {
			day = int(args[1].ToNumber())
		}
	case "fullyear":
		if len(args) > 0 {
			year = int(args[0].ToNumber())
		}
		if len(args) > 1 {
			month = time.Month(int(args[1].ToNumber()) + 1)
		}
		if len(args) > 2 {
			day = int(args[2].ToNumber())
		}
	case "hours":
		if len(args) > 0 {
			hour = int(args[0].ToNumber())
		}
		if len(args) > 1 {
			min = int(args[1].ToNumber())
		}
		if len(args) > 2 {
			sec = int(args[2].ToNumber())
		}
		if len(args) > 3 {
			nsec = int(args[3].ToNumber()) * 1e6
		}
	case "minutes":
		if len(args) > 0 {
			min = int(args[0].ToNumber())
		}
		if len(args) > 1 {
			sec = int(args[1].ToNumber())
		}
		if len(args) > 2 {
			nsec = int(args[2].ToNumber()) * 1e6
		}
	case "seconds":
		if len(args) > 0 {
			sec = int(args[0].ToNumber())
		}
		if len(args) > 1 {
			nsec = int(args[1].ToNumber()) * 1e6
		}
	case "milliseconds":
		if len(args) > 0 {
			nsec = int(args[0].ToNumber()) * 1e6
		}
	case "year":
		if len(args) > 0 {
			y := args[0].ToNumber()
			year = int(y)
			if y >= 0 && y <= 99 {
				year += 1900
			}
		}
	}

	newT := time.Date(year, month, day, hour, min, sec, nsec, t.Location())
	return clampToValidRange(timeToMs(newT))
}

// dateFromSetterArgsUTC is the UTC variant of dateFromSetterArgs.
func dateFromSetterArgsUTC(ms float64, args []JSValue, pattern string) float64 {
	if !isValidTime(ms) {
		return ms
	}

	t := msToTimeUTC(ms)
	year, month, day := t.Date()
	hour, min, sec := t.Clock()
	nsec := t.Nanosecond()

	switch pattern {
	case "day":
		if len(args) > 0 {
			day = int(args[0].ToNumber())
		}
	case "month":
		if len(args) > 0 {
			month = time.Month(int(args[0].ToNumber()) + 1)
		}
		if len(args) > 1 {
			day = int(args[1].ToNumber())
		}
	case "fullyear":
		if len(args) > 0 {
			year = int(args[0].ToNumber())
		}
		if len(args) > 1 {
			month = time.Month(int(args[1].ToNumber()) + 1)
		}
		if len(args) > 2 {
			day = int(args[2].ToNumber())
		}
	case "hours":
		if len(args) > 0 {
			hour = int(args[0].ToNumber())
		}
		if len(args) > 1 {
			min = int(args[1].ToNumber())
		}
		if len(args) > 2 {
			sec = int(args[2].ToNumber())
		}
		if len(args) > 3 {
			nsec = int(args[3].ToNumber()) * 1e6
		}
	case "minutes":
		if len(args) > 0 {
			min = int(args[0].ToNumber())
		}
		if len(args) > 1 {
			sec = int(args[1].ToNumber())
		}
		if len(args) > 2 {
			nsec = int(args[2].ToNumber()) * 1e6
		}
	case "seconds":
		if len(args) > 0 {
			sec = int(args[0].ToNumber())
		}
		if len(args) > 1 {
			nsec = int(args[1].ToNumber()) * 1e6
		}
	case "milliseconds":
		if len(args) > 0 {
			nsec = int(args[0].ToNumber()) * 1e6
		}
	}

	newT := time.Date(year, month, day, hour, min, sec, nsec, time.UTC)
	return clampToValidRange(timeToMs(newT))
}

// =====================================================================
// Getter methods (local time)
// =====================================================================

// dateProtoFuncGetDate implements Date.prototype.getDate() [1-31]
func dateProtoFuncGetDate(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	return JSValueEncode(jsNumber(float64(msToTime(ms).Day())))
}

// dateProtoFuncGetDay implements Date.prototype.getDay() [0=Sun, 6=Sat]
func dateProtoFuncGetDay(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	return JSValueEncode(jsNumber(float64(msToTime(ms).Weekday())))
}

// dateProtoFuncGetFullYear implements Date.prototype.getFullYear()
func dateProtoFuncGetFullYear(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	return JSValueEncode(jsNumber(float64(msToTime(ms).Year())))
}

// dateProtoFuncGetHours implements Date.prototype.getHours() [0-23]
func dateProtoFuncGetHours(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	return JSValueEncode(jsNumber(float64(msToTime(ms).Hour())))
}

// dateProtoFuncGetMilliSeconds implements Date.prototype.getMilliseconds() [0-999]
func dateProtoFuncGetMilliSeconds(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	return JSValueEncode(jsNumber(float64(msToTime(ms).Nanosecond() / 1e6)))
}

// dateProtoFuncGetMinutes implements Date.prototype.getMinutes() [0-59]
func dateProtoFuncGetMinutes(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	return JSValueEncode(jsNumber(float64(msToTime(ms).Minute())))
}

// dateProtoFuncGetMonth implements Date.prototype.getMonth() [0-11]
func dateProtoFuncGetMonth(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	return JSValueEncode(jsNumber(float64(msToTime(ms).Month() - 1)))
}

// dateProtoFuncGetSeconds implements Date.prototype.getSeconds() [0-59]
func dateProtoFuncGetSeconds(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	return JSValueEncode(jsNumber(float64(msToTime(ms).Second())))
}

// dateProtoFuncGetTime implements Date.prototype.getTime() — returns internal ms
func dateProtoFuncGetTime(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	return JSValueEncode(jsNumber(ms))
}

// dateProtoFuncGetTimezoneOffset implements Date.prototype.getTimezoneOffset()
// Returns minutes offset from UTC (positive = behind UTC)
func dateProtoFuncGetTimezoneOffset(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	t := msToTime(ms)
	_, offset := t.Zone()
	return JSValueEncode(jsNumber(float64(-offset / 60)))
}

// dateProtoFuncGetYear implements the deprecated Date.prototype.getYear()
// Returns year - 1900
func dateProtoFuncGetYear(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	return JSValueEncode(jsNumber(float64(msToTime(ms).Year() - 1900)))
}

// =====================================================================
// Getter methods (UTC)
// =====================================================================

func dateProtoFuncGetUTCDate(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	return JSValueEncode(jsNumber(float64(msToTimeUTC(ms).Day())))
}

func dateProtoFuncGetUTCDay(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	return JSValueEncode(jsNumber(float64(msToTimeUTC(ms).Weekday())))
}

func dateProtoFuncGetUTCFullYear(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	return JSValueEncode(jsNumber(float64(msToTimeUTC(ms).Year())))
}

func dateProtoFuncGetUTCHours(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	return JSValueEncode(jsNumber(float64(msToTimeUTC(ms).Hour())))
}

func dateProtoFuncGetUTCMilliseconds(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	return JSValueEncode(jsNumber(float64(msToTimeUTC(ms).Nanosecond() / 1e6)))
}

func dateProtoFuncGetUTCMinutes(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	return JSValueEncode(jsNumber(float64(msToTimeUTC(ms).Minute())))
}

func dateProtoFuncGetUTCMonth(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	return JSValueEncode(jsNumber(float64(msToTimeUTC(ms).Month() - 1)))
}

func dateProtoFuncGetUTCSeconds(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(jsNumber(math.NaN()))
	}
	return JSValueEncode(jsNumber(float64(msToTimeUTC(ms).Second())))
}

// =====================================================================
// Setter methods (local time)
// =====================================================================

func dateProtoFuncSetDate(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	args := callFrame.Arguments()
	newMs := dateFromSetterArgs(ms, args, "day")
	updateDateInstance(globalObject, callFrame.ThisValue(), newMs)
	return JSValueEncode(jsNumber(newMs))
}

func dateProtoFuncSetFullYear(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	args := callFrame.Arguments()
	newMs := dateFromSetterArgs(ms, args, "fullyear")
	updateDateInstance(globalObject, callFrame.ThisValue(), newMs)
	return JSValueEncode(jsNumber(newMs))
}

func dateProtoFuncSetHours(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	args := callFrame.Arguments()
	newMs := dateFromSetterArgs(ms, args, "hours")
	updateDateInstance(globalObject, callFrame.ThisValue(), newMs)
	return JSValueEncode(jsNumber(newMs))
}

func dateProtoFuncSetMilliSeconds(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	args := callFrame.Arguments()
	newMs := dateFromSetterArgs(ms, args, "milliseconds")
	updateDateInstance(globalObject, callFrame.ThisValue(), newMs)
	return JSValueEncode(jsNumber(newMs))
}

func dateProtoFuncSetMinutes(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	args := callFrame.Arguments()
	newMs := dateFromSetterArgs(ms, args, "minutes")
	updateDateInstance(globalObject, callFrame.ThisValue(), newMs)
	return JSValueEncode(jsNumber(newMs))
}

func dateProtoFuncSetMonth(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	args := callFrame.Arguments()
	newMs := dateFromSetterArgs(ms, args, "month")
	updateDateInstance(globalObject, callFrame.ThisValue(), newMs)
	return JSValueEncode(jsNumber(newMs))
}

func dateProtoFuncSetSeconds(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	args := callFrame.Arguments()
	newMs := dateFromSetterArgs(ms, args, "seconds")
	updateDateInstance(globalObject, callFrame.ThisValue(), newMs)
	return JSValueEncode(jsNumber(newMs))
}

func dateProtoFuncSetTime(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	args := callFrame.Arguments()
	newMs := math.NaN()
	if len(args) > 0 {
		newMs = clampToValidRange(args[0].ToNumber())
	}
	updateDateInstance(globalObject, callFrame.ThisValue(), newMs)
	return JSValueEncode(jsNumber(newMs))
}

func dateProtoFuncSetYear(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	args := callFrame.Arguments()
	newMs := dateFromSetterArgs(ms, args, "year")
	updateDateInstance(globalObject, callFrame.ThisValue(), newMs)
	return JSValueEncode(jsNumber(newMs))
}

// =====================================================================
// Setter methods (UTC)
// =====================================================================

func dateProtoFuncSetUTCDate(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	args := callFrame.Arguments()
	newMs := dateFromSetterArgsUTC(ms, args, "day")
	updateDateInstance(globalObject, callFrame.ThisValue(), newMs)
	return JSValueEncode(jsNumber(newMs))
}

func dateProtoFuncSetUTCFullYear(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	args := callFrame.Arguments()
	newMs := dateFromSetterArgsUTC(ms, args, "fullyear")
	updateDateInstance(globalObject, callFrame.ThisValue(), newMs)
	return JSValueEncode(jsNumber(newMs))
}

func dateProtoFuncSetUTCHours(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	args := callFrame.Arguments()
	newMs := dateFromSetterArgsUTC(ms, args, "hours")
	updateDateInstance(globalObject, callFrame.ThisValue(), newMs)
	return JSValueEncode(jsNumber(newMs))
}

func dateProtoFuncSetUTCMilliseconds(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	args := callFrame.Arguments()
	newMs := dateFromSetterArgsUTC(ms, args, "milliseconds")
	updateDateInstance(globalObject, callFrame.ThisValue(), newMs)
	return JSValueEncode(jsNumber(newMs))
}

func dateProtoFuncSetUTCMinutes(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	args := callFrame.Arguments()
	newMs := dateFromSetterArgsUTC(ms, args, "minutes")
	updateDateInstance(globalObject, callFrame.ThisValue(), newMs)
	return JSValueEncode(jsNumber(newMs))
}

func dateProtoFuncSetUTCMonth(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	args := callFrame.Arguments()
	newMs := dateFromSetterArgsUTC(ms, args, "month")
	updateDateInstance(globalObject, callFrame.ThisValue(), newMs)
	return JSValueEncode(jsNumber(newMs))
}

func dateProtoFuncSetUTCSeconds(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	args := callFrame.Arguments()
	newMs := dateFromSetterArgsUTC(ms, args, "seconds")
	updateDateInstance(globalObject, callFrame.ThisValue(), newMs)
	return JSValueEncode(jsNumber(newMs))
}

// updateDateInstance sets the internal time value on a Date object.
func updateDateInstance(globalObject *JSGlobalObject, thisValue JSValue, ms float64) {
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
		return
	}
	if di := dynamicDowncast[DateInstance](thisObj); di != nil {
		di.SetInternalValue(ms)
	}
}

// =====================================================================
// String conversion methods
// =====================================================================

// dateProtoFuncToString implements Date.prototype.toString()
func dateProtoFuncToString(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(NewJSValueString("Invalid Date"))
	}
	t := msToTime(ms)
	// Format: "Tue Jul 19 2026 10:22:31 GMT+0800 (CST)"
	zone, offset := t.Zone()
	offsetStr := formatOffset(offset)
	return JSValueEncode(NewJSValueString(
		t.Format("Mon Jan 2 2006 15:04:05") + " GMT" + offsetStr + " (" + zone + ")"))
}

// dateProtoFuncToDateString implements Date.prototype.toDateString()
func dateProtoFuncToDateString(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(NewJSValueString("Invalid Date"))
	}
	t := msToTime(ms)
	return JSValueEncode(NewJSValueString(t.Format("Mon Jan 2 2006")))
}

// dateProtoFuncToTimeString implements Date.prototype.toTimeString()
func dateProtoFuncToTimeString(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(NewJSValueString("Invalid Date"))
	}
	t := msToTime(ms)
	zone, offset := t.Zone()
	offsetStr := formatOffset(offset)
	return JSValueEncode(NewJSValueString(
		t.Format("15:04:05") + " GMT" + offsetStr + " (" + zone + ")"))
}

// dateProtoFuncToUTCString implements Date.prototype.toUTCString()
func dateProtoFuncToUTCString(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return JSValueEncode(NewJSValueString("Invalid Date"))
	}
	t := msToTimeUTC(ms)
	return JSValueEncode(NewJSValueString(t.Format("Mon, 02 Jan 2006 15:04:05 GMT")))
}

// dateProtoFuncToISOString implements Date.prototype.toISOString()
func dateProtoFuncToISOString(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	if !isValidTime(ms) {
		return throwVMTypeError(globalObject, ThrowScope{vm: globalObject.VM()},
			"Invalid Date: Date.prototype.toISOString called on non-existent Date")
	}
	t := msToTimeUTC(ms)
	return JSValueEncode(NewJSValueString(t.Format("2006-01-02T15:04:05.000Z")))
}

// dateProtoFuncToJSON implements Date.prototype.toJSON(key)
// ES 21.4.4.38 Date.prototype.toJSON ( key )
func dateProtoFuncToJSON(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	// ES: 1. Let O be ? ToObject(this value)
	thisObj := callFrame.ThisValue().ToObject(globalObject)
	if thisObj == nil {
		return EncodedJSValue()
	}

	// ES: 2. Let tv be ? ToPrimitive(O, hint Number)
	tv := callFrame.ThisValue().ToPrimitive(NoPreference)

	// ES: 3. If Type(tv) is Number and tv is not finite, return null
	if tv.IsNumber() {
		n := tv.ToNumber()
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return JSValueEncode(jsNull())
		}
	}

	// ES: 4. Let toISO be ? Get(O, "toISOString")
	toISO := thisObj.Get(globalObject, NewPropertyName("toISOString"))

	// ES: 5. If IsCallable(toISO) is false, throw TypeError
	if !toISO.IsCallable() {
		return throwVMTypeError(globalObject, ThrowScope{vm: globalObject.VM()},
			"toISOString is not callable")
	}

	// ES: 6. Return ? Call(toISO, O)
	callData := getCallDataInline(toISO)
	return call(globalObject, toISO, callData, callFrame.ThisValue(), nil)
}

// dateProtoFuncToLocaleString implements Date.prototype.toLocaleString()
func dateProtoFuncToLocaleString(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	_ = callFrame
	return dateProtoFuncToString(globalObject, callFrame)
}

// dateProtoFuncToLocaleDateString implements Date.prototype.toLocaleDateString()
func dateProtoFuncToLocaleDateString(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	_ = callFrame
	return dateProtoFuncToDateString(globalObject, callFrame)
}

// dateProtoFuncToLocaleTimeString implements Date.prototype.toLocaleTimeString()
func dateProtoFuncToLocaleTimeString(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	_ = callFrame
	return dateProtoFuncToTimeString(globalObject, callFrame)
}

// dateProtoFuncValueOf implements Date.prototype.valueOf() — returns internal ms
func dateProtoFuncValueOf(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	ms := thisTimeValue(globalObject, callFrame.ThisValue())
	return JSValueEncode(jsNumber(ms))
}

// dateProtoFuncToPrimitiveSymbol implements Date.prototype[@@toPrimitive](hint)
// ES 21.4.4.42 Date.prototype [ @@toPrimitive ] ( hint )
func dateProtoFuncToPrimitiveSymbol(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	// ES: 1. Let O be the this value.
	thisValue := callFrame.ThisValue()

	// ES: 2. If Type(O) is not Object, throw TypeError
	if !thisValue.IsObject() {
		return throwVMTypeError(globalObject, ThrowScope{vm: globalObject.VM()},
			"Date.prototype[@@toPrimitive] requires an object")
	}

	// ES: 3. If hint is "string" or "default", try string first; if "number", try number first
	hint := callFrame.Argument(0).ToString()

	var tryFirst PreferredPrimitiveType
	var trySecond PreferredPrimitiveType
	switch hint {
	case "string":
		tryFirst = PreferString
		trySecond = PreferNumber
	case "number":
		tryFirst = PreferNumber
		trySecond = PreferString
	default: // "default"
		tryFirst = PreferString
		trySecond = PreferNumber
	}

	// ES: 4. Let exoticToPrim be ? GetMethod(O, @@toPrimitive) — this is the method itself, skip

	// ES: 5-6. Try ToPrimitive with preferred type
	result := thisValue.ToObject(globalObject).DefaultValue(globalObject, tryFirst)
	if result.IsUndefined() {
		result = thisValue.ToObject(globalObject).DefaultValue(globalObject, trySecond)
	}
	return JSValueEncode(result)
}

// =====================================================================
// Utility
// =====================================================================

// formatOffset formats a timezone offset in seconds to ±HHMM string.
func formatOffset(offset int) string {
	if offset == 0 {
		return "+0000"
	}
	absOffset := offset
	sign := "+"
	if offset < 0 {
		sign = "-"
		absOffset = -offset
	}
	hours := absOffset / 3600
	minutes := (absOffset % 3600) / 60
	return fmt.Sprintf("%s%02d%02d", sign, hours, minutes)
}
