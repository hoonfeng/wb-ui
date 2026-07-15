// Translation of: Source/WebCore/dom/FormDataEvent.h
//                  Source/WebCore/dom/FormDataEvent.cpp
//                  Source/WebCore/dom/FormDataEvent.idl
// Completeness: 75%
//
// FormDataEvent is fired during form submission to allow scripts to inspect
// and modify the form data before it is sent. It carries a reference to the
// DOMFormData being submitted.
//
// In WebKit, FormDataEvent derives from Event and holds a
// RefPtr<DOMFormData>. This port holds the DOMFormData by reference; since
// DOMFormData lives in the html5 package (which depends on dom), we store it
// as an interface{} to avoid an import cycle.

package dom

// FormDataEventInit mirrors WebCore::FormDataEvent::Init (the EventInit-derived
// dictionary used by the FormDataEvent(type, init) constructor).
type FormDataEventInit struct {
	EventInit
	FormData interface{}
}

// FormDataEvent is the Go translation of WebCore::FormDataEvent. It is
// dispatched when a form's submit algorithm runs, allowing listeners to
// modify the form data. The event type string is "formdata".
type FormDataEvent struct {
	baseEvent
	formData interface{}
}

// FormData event type constants.
const (
	EventFormData = "formdata"
)

// NewFormDataEvent constructs a FormDataEvent, mirroring
// FormDataEvent::create(formData). The event bubbles and is cancelable.
func NewFormDataEvent(formData interface{}) *FormDataEvent {
	return &FormDataEvent{
		baseEvent: newBaseEvent(EventFormData, true, true, true, true),
		formData:  formData,
	}
}

// NewFormDataEventFromInit constructs a FormDataEvent from a FormDataEventInit
// dictionary, mirroring FormDataEvent::create(type, init).
func NewFormDataEventFromInit(typ string, init FormDataEventInit) *FormDataEvent {
	return &FormDataEvent{
		baseEvent: newBaseEvent(typ, init.Bubbles, init.Cancelable, init.Composed, false),
		formData:  init.FormData,
	}
}

// FormData returns the DOMFormData associated with this event. The concrete
// type is *html5.DOMFormData; it is returned as interface{} to avoid an import
// cycle between dom and html5.
func (e *FormDataEvent) FormData() interface{} { return e.formData }
