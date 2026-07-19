// Package bytecompiler provides the bytecode generator.
// This file corresponds to WebKit ProfileTypeBytecodeFlag.h.

package bytecompiler

// ProfileTypeBytecodeFlag corresponds to the JSC enum of the same name.
type ProfileTypeBytecodeFlag int

const (
	ProfileTypeBytecodeClosureVar          ProfileTypeBytecodeFlag = iota
	ProfileTypeBytecodeLocallyResolved
	ProfileTypeBytecodeDoesNotHaveGlobalID
	ProfileTypeBytecodeFunctionArgument
	ProfileTypeBytecodeFunctionReturnStatement
)
