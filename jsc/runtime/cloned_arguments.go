// ClonedArguments corresponds to JSC::ClonedArguments.
package runtime

// ClonedArguments corresponds to JSC::ClonedArguments.
// Holds a copy of function arguments (used when "arguments" is accessed in a function).
type ClonedArguments struct {
	JSObject
	argumentCount int
	arguments     []JSValue
}

const ClonedArgumentsStructureFlags uint32 = JSObjectStructureFlags

func NewClonedArguments(vm *VM, structure *Structure, args []JSValue) *ClonedArguments {
	obj := &ClonedArguments{
		argumentCount: len(args),
		arguments:     args,
	}
	obj.structureID = structure.structureID
	obj.typ = ClonedArgumentsType
	obj.cellState = DefinitelyWhite
	obj.properties = make(map[string]JSValue)
	return obj
}

func (a *ClonedArguments) Length() int        { return a.argumentCount }
func (a *ClonedArguments) Argument(i int) JSValue {
	if i >= 0 && i < len(a.arguments) {
		return a.arguments[i]
	}
	return JSValueUndefined
}
