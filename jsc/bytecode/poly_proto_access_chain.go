// PolyProtoAccessChain - polymorphic prototype access chain (JIT IC)
package bytecode

type PolyProtoAccessChain struct {
	chain []uint64 // JSCell* pointers
}

func (c *PolyProtoAccessChain) IsEmpty() bool { return len(c.chain) == 0 }
func (c *PolyProtoAccessChain) Size() int { return len(c.chain) }
