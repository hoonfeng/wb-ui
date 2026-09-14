// Translation of: Source/WebCore/dom/KeyboardEvent.h
//                  Source/WebCore/dom/KeyboardEvent.cpp
//                  Source/WebCore/dom/KeyboardEventInit.h
// Completeness: 75%
// Simplifications:
//   - the UIEventWithKeyState intermediate layer is flattened: KeyboardEvent embeds
//     baseEvent directly and carries the modifier fields itself
//   - PlatformKeyboardEvent plumbing is omitted; events are constructed from dictionaries
//   - keyCode/charCode legacy integer codes are preserved as plain ints (no mapping table)
//   - the keys "Fn"/"CapsLock"/"SymbolLock" modifiers are reported via getModifierState but
//     have no backing field (always false) since this port has no platform keyboard adapter

package dom

// KeyLocationCode mirrors WebCore::KeyboardEvent::KeyLocationCode.
type KeyLocationCode int

// Key location constants, mirroring KeyboardEvent::DOM_KEY_LOCATION_*.
const (
	DOMKeyLocationStandard KeyLocationCode = 0
	DOMKeyLocationLeft     KeyLocationCode = 1
	DOMKeyLocationRight    KeyLocationCode = 2
	DOMKeyLocationNumpad   KeyLocationCode = 3
)

// Keyboard event type constants.
const (
	EventKeyDown  = "keydown"
	EventKeyUp    = "keyup"
	EventKeyPress = "keypress"
)

type KeyboardEventInit struct {
	EventInit
	Key            string
	Code           string
	KeyIdentifier  string
	Location       KeyLocationCode
	Repeat         bool
	IsComposing    bool
	CtrlKey        bool
	AltKey         bool
	ShiftKey       bool
	MetaKey        bool
	KeyCode        int
	CharCode       int
	AltGraphKey    bool
	CapsLockKey    bool
	FnKey          bool
	NumLockKey     bool
	SymbolKey      bool
	SymbolLockKey  bool
	OSKey          bool
}

// KeyboardEvent is the Go translation of WebCore::KeyboardEvent. It embeds baseEvent for
// the common Event machinery and adds the key/code/location/modifier fields. Modifier
// state queries go through GetModifierState so callers can use the DOM key names.
type KeyboardEvent struct {
	baseEvent
	key              string
	code             string
	keyIdentifier    string
	location         KeyLocationCode
	repeat           bool
	isComposing      bool
	ctrlKey          bool
	altKey           bool
	shiftKey         bool
	metaKey          bool
	altGraphKey      bool
	capsLockKey      bool
	fnKey            bool
	numLockKey       bool
	symbolKey        bool
	symbolLockKey    bool
	osKey            bool
	keyCode          int
	charCode         int
}

// NewKeyboardEvent constructs a KeyboardEvent, mirroring KeyboardEvent::create(type,
// canBubble, isCancelable, isComposed).
func NewKeyboardEvent(typ string, canBubble, cancelable, composed bool) *KeyboardEvent {
	return &KeyboardEvent{
		baseEvent: newBaseEvent(typ, canBubble, cancelable, composed, true),
	}
}

// NewKeyboardEventFromInit constructs a KeyboardEvent from a KeyboardEventInit dictionary,
// mirroring KeyboardEvent::create(type, init).
func NewKeyboardEventFromInit(typ string, init KeyboardEventInit) *KeyboardEvent {
	return &KeyboardEvent{
		baseEvent:     newBaseEvent(typ, init.Bubbles, init.Cancelable, init.Composed, false),
		key:           init.Key,
		code:          init.Code,
		keyIdentifier: init.KeyIdentifier,
		location:      init.Location,
		repeat:        init.Repeat,
		isComposing:   init.IsComposing,
		ctrlKey:       init.CtrlKey,
		altKey:        init.AltKey,
		shiftKey:      init.ShiftKey,
		metaKey:       init.MetaKey,
		altGraphKey:   init.AltGraphKey,
		capsLockKey:   init.CapsLockKey,
		fnKey:         init.FnKey,
		numLockKey:    init.NumLockKey,
		symbolKey:     init.SymbolKey,
		symbolLockKey: init.SymbolLockKey,
		osKey:         init.OSKey,
		keyCode:       init.KeyCode,
		charCode:      init.CharCode,
	}
}

// initKeyboardEvent re-initialises the event, mirroring KeyboardEvent::initKeyboardEvent().
func (k *KeyboardEvent) initKeyboardEvent(typ string, canBubble, cancelable bool, key, code string, location KeyLocationCode, ctrlKey, altKey, shiftKey, metaKey bool) {
	k.initEvent(typ, canBubble, cancelable)
	k.key = key
	k.code = code
	k.location = location
	k.ctrlKey, k.altKey, k.shiftKey, k.metaKey = ctrlKey, altKey, shiftKey, metaKey
}

// Attribute accessors, mirroring KeyboardEvent::key()/code()/location()/repeat()/...
func (k *KeyboardEvent) Key() string             { return k.key }
func (k *KeyboardEvent) Code() string            { return k.code }
func (k *KeyboardEvent) KeyIdentifier() string  { return k.keyIdentifier }
func (k *KeyboardEvent) Location() KeyLocationCode { return k.location }
func (k *KeyboardEvent) Repeat() bool           { return k.repeat }
func (k *KeyboardEvent) IsComposing() bool      { return k.isComposing }
func (k *KeyboardEvent) KeyCode() int            { return k.keyCode }
func (k *KeyboardEvent) CharCode() int          { return k.charCode }

// Modifier accessors, mirroring UIEventWithKeyState modifier getters.
func (k *KeyboardEvent) CtrlKey() bool     { return k.ctrlKey }
func (k *KeyboardEvent) AltKey() bool      { return k.altKey }
func (k *KeyboardEvent) ShiftKey() bool     { return k.shiftKey }
func (k *KeyboardEvent) MetaKey() bool     { return k.metaKey }
func (k *KeyboardEvent) AltGraphKey() bool { return k.altGraphKey }
func (k *KeyboardEvent) CapsLockKey() bool { return k.capsLockKey }
func (k *KeyboardEvent) FnKey() bool       { return k.fnKey }
func (k *KeyboardEvent) NumLockKey() bool  { return k.numLockKey }
func (k *KeyboardEvent) SymbolKey() bool  { return k.symbolKey }
func (k *KeyboardEvent) SymbolLockKey() bool { return k.symbolLockKey }
func (k *KeyboardEvent) OSKey() bool       { return k.osKey }

// GetModifierState reports whether the named modifier is active, mirroring
// KeyboardEvent::getModifierState(key).
func (k *KeyboardEvent) GetModifierState(key string) bool {
	switch key {
	case "Control":
		return k.ctrlKey
	case "Alt":
		return k.altKey
	case "Shift":
		return k.shiftKey
	case "Meta":
		return k.metaKey
	case "AltGraph":
		return k.altGraphKey
	case "CapsLock":
		return k.capsLockKey
	case "Fn":
		return k.fnKey
	case "NumLock":
		return k.numLockKey
	case "Symbol":
		return k.symbolKey
	case "SymbolLock":
		return k.symbolLockKey
	case "OS":
		return k.osKey
	}
	return false
}
