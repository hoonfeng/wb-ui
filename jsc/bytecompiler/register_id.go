// RegisterID.h Go 翻译
package bytecompiler

// VirtualRegister 虚拟寄存器
type VirtualRegister struct {
	OffsetVal int
}

func NewVirtualRegister(offset int) VirtualRegister {
	return VirtualRegister{OffsetVal: offset}
}

func (v VirtualRegister) Offset() int     { return v.OffsetVal }
func (v VirtualRegister) IsValid() bool   { return v.OffsetVal != -1 }
func (v VirtualRegister) IsConstant() bool { return v.OffsetVal >= 0 }

// RegisterID 寄存器 ID
type RegisterID struct {
	refCount    int
	VirtualReg  VirtualRegister
	IsTemporary bool
	DidSetIndex bool
}

func NewRegisterID() *RegisterID {
	return &RegisterID{refCount: 0, IsTemporary: false}
}

func NewRegisterIDFromVirtual(vr VirtualRegister) *RegisterID {
	return &RegisterID{refCount: 0, VirtualReg: vr, IsTemporary: false, DidSetIndex: true}
}

func NewRegisterIDFromIndex(index int) *RegisterID {
	return &RegisterID{refCount: 0, VirtualReg: NewVirtualRegister(index), IsTemporary: false, DidSetIndex: true}
}

func (r *RegisterID) SetIndex(vr VirtualRegister) { r.DidSetIndex = true; r.VirtualReg = vr }
func (r *RegisterID) SetTemporary()                { r.IsTemporary = true }
func (r *RegisterID) Index() int                    { return r.VirtualReg.Offset() }
func (r *RegisterID) VirtualRegister() VirtualRegister { return r.VirtualReg }
func (r *RegisterID) IsTemporaryVal() bool          { return r.IsTemporary }
func (r *RegisterID) Ref()                   { r.refCount++ }
func (r *RegisterID) Deref()                 { r.refCount-- }
func (r *RegisterID) RefCount() int          { return r.refCount }
