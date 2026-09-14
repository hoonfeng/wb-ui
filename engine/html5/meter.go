package html5

import (
	"strconv"

	"wb-ui/engine/dom"
)

// --- HTMLMeterElement ---

// HTMLMeterElement wraps a <meter> element. Mirrors
// WebCore::HTMLMeterElement.
type HTMLMeterElement struct {
	El *dom.Element
}

// ToMeterElement wraps an element as an HTMLMeterElement.
func ToMeterElement(el *dom.Element) (HTMLMeterElement, bool) {
	if el == nil || el.LocalName() != "meter" {
		return HTMLMeterElement{}, false
	}
	return HTMLMeterElement{El: el}, true
}

// Value returns the value attribute parsed as float64, defaulting to 0.
func (m HTMLMeterElement) Value() float64 {
	return attrFloat(m.El, "value")
}

// SetValue sets the value attribute.
func (m HTMLMeterElement) SetValue(v float64) {
	m.El.SetAttribute("value", strconv.FormatFloat(v, 'f', -1, 64))
}

// Min returns the min attribute, defaulting to 0.
func (m HTMLMeterElement) Min() float64 {
	if !m.El.HasAttribute("min") {
		return 0
	}
	return attrFloat(m.El, "min")
}

// SetMin sets the min attribute.
func (m HTMLMeterElement) SetMin(v float64) {
	m.El.SetAttribute("min", strconv.FormatFloat(v, 'f', -1, 64))
}

// Max returns the max attribute, defaulting to 1.
func (m HTMLMeterElement) Max() float64 {
	if !m.El.HasAttribute("max") {
		return 1
	}
	return attrFloat(m.El, "max")
}

// SetMax sets the max attribute.
func (m HTMLMeterElement) SetMax(v float64) {
	m.El.SetAttribute("max", strconv.FormatFloat(v, 'f', -1, 64))
}

// Low returns the low attribute, defaulting to the min value.
func (m HTMLMeterElement) Low() float64 {
	if !m.El.HasAttribute("low") {
		return m.Min()
	}
	return attrFloat(m.El, "low")
}

// SetLow sets the low attribute.
func (m HTMLMeterElement) SetLow(v float64) {
	m.El.SetAttribute("low", strconv.FormatFloat(v, 'f', -1, 64))
}

// High returns the high attribute, defaulting to the max value.
func (m HTMLMeterElement) High() float64 {
	if !m.El.HasAttribute("high") {
		return m.Max()
	}
	return attrFloat(m.El, "high")
}

// SetHigh sets the high attribute.
func (m HTMLMeterElement) SetHigh(v float64) {
	m.El.SetAttribute("high", strconv.FormatFloat(v, 'f', -1, 64))
}

// Optimum returns the optimum attribute, defaulting to the midpoint of min
// and max.
func (m HTMLMeterElement) Optimum() float64 {
	if !m.El.HasAttribute("optimum") {
		return (m.Min() + m.Max()) / 2
	}
	return attrFloat(m.El, "optimum")
}

// SetOptimum sets the optimum attribute.
func (m HTMLMeterElement) SetOptimum(v float64) {
	m.El.SetAttribute("optimum", strconv.FormatFloat(v, 'f', -1, 64))
}

// Position returns the value clamped to [min, max] and normalized to [0, 1],
// or -1 if the meter's range is degenerate (min == max).
func (m HTMLMeterElement) Position() float64 {
	min := m.Min()
	max := m.Max()
	if min == max {
		return -1
	}
	v := m.Value()
	if v < min {
		v = min
	}
	if v > max {
		v = max
	}
	return (v - min) / (max - min)
}

// --- HTMLProgressElement ---

// HTMLProgressElement wraps a <progress> element. Mirrors
// WebCore::HTMLProgressElement.
type HTMLProgressElement struct {
	El *dom.Element
}

// ToProgressElement wraps an element as an HTMLProgressElement.
func ToProgressElement(el *dom.Element) (HTMLProgressElement, bool) {
	if el == nil || el.LocalName() != "progress" {
		return HTMLProgressElement{}, false
	}
	return HTMLProgressElement{El: el}, true
}

// Value returns the value attribute parsed as float64, defaulting to 0.
func (p HTMLProgressElement) Value() float64 {
	return attrFloat(p.El, "value")
}

// SetValue sets the value attribute.
func (p HTMLProgressElement) SetValue(v float64) {
	p.El.SetAttribute("value", strconv.FormatFloat(v, 'f', -1, 64))
}

// Max returns the max attribute, defaulting to 1.
func (p HTMLProgressElement) Max() float64 {
	if !p.El.HasAttribute("max") {
		return 1
	}
	return attrFloat(p.El, "max")
}

// SetMax sets the max attribute.
func (p HTMLProgressElement) SetMax(v float64) {
	p.El.SetAttribute("max", strconv.FormatFloat(v, 'f', -1, 64))
}

// Position returns the progress as a ratio in [0, 1], or -1 if the progress
// is indeterminate (no value attribute).
func (p HTMLProgressElement) Position() float64 {
	if !p.El.HasAttribute("value") {
		return -1
	}
	max := p.Max()
	if max == 0 {
		return 0
	}
	v := p.Value()
	if v > max {
		v = max
	}
	return v / max
}

// Indeterminate reports whether the progress bar is in the indeterminate state
// (no value attribute present), mirroring HTMLProgressElement::isIndeterminate().
func (p HTMLProgressElement) Indeterminate() bool {
	return !p.El.HasAttribute("value")
}
