// ObjectPropertyCondition / ObjectPropertyConditionSet / PropertyCondition
// Describe property access conditions for speculation.
package bytecode

type PropertyCondition struct {
	kind      uint8 // 0=Equivalence, 1=HasInstance, 2=Presence, 3=Absence, 4=AbsenceOfSetter
	offset    uint32
	object    uint64 // JSCell*
}

type ObjectPropertyCondition struct {
	PropertyCondition
	object uint64
}

type ObjectPropertyConditionSet struct {
	conditions []ObjectPropertyCondition
}

func NewObjectPropertyConditionSet() *ObjectPropertyConditionSet {
	return &ObjectPropertyConditionSet{}
}

func (s *ObjectPropertyConditionSet) IsEmpty() bool { return len(s.conditions) == 0 }
func (s *ObjectPropertyConditionSet) Size() int { return len(s.conditions) }
func (s *ObjectPropertyConditionSet) Add(c ObjectPropertyCondition) {
	s.conditions = append(s.conditions, c)
}
