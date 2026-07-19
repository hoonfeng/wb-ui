// DirectArguments corresponds to JSC::DirectArguments.
package runtime

// DirectArguments corresponds to JSC::DirectArguments.
// Holds the arguments object for functions that use simple (non-mapped) arguments.
type DirectArguments struct {
	JSObject
	argumentCount int
	arguments     []JSValue
}

const DirectArgumentsStructureFlags uint32 = JSObjectStructureFlags

func NewDirectArguments(vm *VM, structure *Structure, args []JSValue) *DirectArguments {
	obj := &DirectArguments{
		argumentCount: len(args),
		arguments:     args,
	}
	obj.structureID = structure.structureID
	obj.typ = DirectArgumentsType
	obj.cellState = DefinitelyWhite
	obj.properties = make(map[string]JSValue)
	return obj
}

func (a *DirectArguments) Length() int        { return a.argumentCount }
func (a *DirectArguments) Argument(i int) JSValue {
	if i >= 0 && i < len(a.arguments) {
		return a.arguments[i]
	}
	return JSValueUndefined
}
