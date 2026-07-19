// ScopedArguments corresponds to JSC::ScopedArguments.
package runtime

// ScopedArguments corresponds to JSC::ScopedArguments.
// Holds the arguments object for functions with captured (scoped) arguments.
type ScopedArguments struct {
	JSObject
	argumentCount int
	arguments     []JSValue
}

const ScopedArgumentsStructureFlags uint32 = JSObjectStructureFlags

func NewScopedArguments(vm *VM, structure *Structure, args []JSValue) *ScopedArguments {
	obj := &ScopedArguments{
		argumentCount: len(args),
		arguments:     args,
	}
	obj.structureID = structure.structureID
	obj.typ = ScopedArgumentsType
	obj.cellState = DefinitelyWhite
	obj.properties = make(map[string]JSValue)
	return obj
}

func (a *ScopedArguments) Length() int        { return a.argumentCount }
func (a *ScopedArguments) Argument(i int) JSValue {
	if i >= 0 && i < len(a.arguments) {
		return a.arguments[i]
	}
	return JSValueUndefined
}
