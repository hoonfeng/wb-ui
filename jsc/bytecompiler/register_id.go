// Package bytecompiler provides the bytecode generator.
// This file corresponds to WebKit RegisterID.h.

package bytecompiler

// VirtualRegister is an int alias for stack/frame register positions.
type VirtualRegister = int

// RegisterID corresponds to WebKit's RegisterID class.
type RegisterID struct {
	refCount      int
	virtualReg    VirtualRegister
	isTemp        bool
	didSetIndex   bool
}

func NewRegisterID() *RegisterID {
	return &RegisterID{}
}

func NewRegisterIDFromVirtual(vr VirtualRegister) *RegisterID {
	return &RegisterID{
		virtualReg:  vr,
		didSetIndex: true,
	}
}

func NewRegisterIDFromIndex(index int) *RegisterID {
	return &RegisterID{
		virtualReg:  index,
		didSetIndex: true,
	}
}

func (r *RegisterID) SetIndex(index VirtualRegister) {
	r.didSetIndex = true
	r.virtualReg = index
}

func (r *RegisterID) SetTemporary()         { r.isTemp = true }
func (r *RegisterID) Index() int            { return r.virtualReg }
func (r *RegisterID) VirtualRegister() VirtualRegister { return r.virtualReg }
func (r *RegisterID) IsTemporary() bool     { return r.isTemp }

func (r *RegisterID) Ref()   { r.refCount++ }
func (r *RegisterID) Deref() {
	r.refCount--
	if r.refCount < 0 {
		panic("RegisterID refCount went negative")
	}
}
func (r *RegisterID) RefCount() int { return r.refCount }
