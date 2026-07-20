// Label.h Go 翻译 - GenericLabel + GenericBoundLabel
package bytecompiler

const invalidLabelLocation int = -1

type BoundLabelType int
const (
	BoundLabelOffset           BoundLabelType = 0
	BoundLabelGeneratorForward BoundLabelType = 1
	BoundLabelGeneratorBackward BoundLabelType = 2
)

// BoundLabel 已绑定标签
type BoundLabel struct {
	Type        BoundLabelType
	Label       *Label
	Target      int
	savedTarget int
}

func NewBoundLabel() BoundLabel {
	return BoundLabel{Type: BoundLabelOffset, Target: 0}
}

func NewBoundLabelWithOffset(offset int) BoundLabel {
	return BoundLabel{Type: BoundLabelOffset, Target: offset}
}

func (b *BoundLabel) TargetOffset() int {
	switch b.Type {
	case BoundLabelOffset:
		return b.Target
	case BoundLabelGeneratorBackward:
		return b.Target
	default:
		return 0
	}
}

func (b *BoundLabel) SaveTarget() int {
	b.savedTarget = b.TargetOffset()
	return b.savedTarget
}

func (b *BoundLabel) CommitTarget() int {
	if b.Type == BoundLabelGeneratorForward && b.Label != nil {
		b.Label.UnresolvedJumps = append(b.Label.UnresolvedJumps, b.savedTarget)
		return 0
	}
	return b.savedTarget
}

// Label 标签
type Label struct {
	refCount        int
	LocationVal     int
	Bound           bool
	UnresolvedJumps []int
}

func NewLabel() *Label {
	return &Label{LocationVal: invalidLabelLocation}
}

func (l *Label) SetLocation(location int) {
	l.LocationVal = location
	l.Bound = true
}

func (l *Label) BindWithOffset(offset int) BoundLabel {
	l.Bound = true
	if !l.IsForward() {
		return NewBoundLabelWithOffset(l.LocationVal - offset)
	}
	l.UnresolvedJumps = append(l.UnresolvedJumps, offset)
	return NewBoundLabel()
}

func (l *Label) Bind() BoundLabel {
	return l.BindWithOffset(0)
}

func (l *Label) Ref()          { l.refCount++ }
func (l *Label) Deref()        { l.refCount-- }
func (l *Label) RefCount() int { return l.refCount }
func (l *Label) HasOneRef() bool { return l.refCount == 1 }
func (l *Label) IsForward() bool { return l.LocationVal == invalidLabelLocation }
func (l *Label) IsBound() bool { return l.Bound }
func (l *Label) Location() int { return l.LocationVal }
