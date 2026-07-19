// JSBigInt corresponds to JSC::JSBigInt.
// Simplified Go implementation using Go's big.Int.
package runtime

import "math/big"

// JSBigInt corresponds to JSC::JSBigInt.
// Represents arbitrary-precision integers (BigInt primitive).
type JSBigInt struct {
	JSCell
	value *big.Int
}

const BigIntStructureFlags uint32 = JSCellStructureFlags

func NewJSBigInt(vm *VM, structure *Structure) *JSBigInt {
	bi := &JSBigInt{
		value: new(big.Int),
	}
	bi.structureID = structure.structureID
	bi.typ = HeapBigIntType
	bi.cellState = DefinitelyWhite
	return bi
}

func NewJSBigIntFromInt64(vm *VM, structure *Structure, n int64) *JSBigInt {
	bi := NewJSBigInt(vm, structure)
	bi.value.SetInt64(n)
	return bi
}

func (bi *JSBigInt) Int64() int64 {
	if bi.value.IsInt64() {
		return bi.value.Int64()
	}
	return 0
}

func (bi *JSBigInt) String() string {
	return bi.value.String()
}

func (bi *JSBigInt) ToNumber() float64 {
	f, _ := new(big.Float).SetInt(bi.value).Float64()
	return f
}
