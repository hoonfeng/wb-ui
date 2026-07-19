// CheckPrivateBrandStatus / SetPrivateBrandStatus / CheckPrivateBrandVariant / SetPrivateBrandVariant
// (JIT IC status tracking for private brand checks)
package bytecode

type CheckPrivateBrandStatus struct{ state uint8 }
type CheckPrivateBrandVariant struct{}
type SetPrivateBrandStatus struct{ state uint8 }
type SetPrivateBrandVariant struct{}
