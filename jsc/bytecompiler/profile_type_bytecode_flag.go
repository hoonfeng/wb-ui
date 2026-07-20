// ProfileTypeBytecodeFlag.h Go 翻译
package bytecompiler

type ProfileTypeBytecodeFlag int

const (
	ProfileTypeBytecodeClosureVar           ProfileTypeBytecodeFlag = 0
	ProfileTypeBytecodeLocallyResolved      ProfileTypeBytecodeFlag = 1
	ProfileTypeBytecodeDoesNotHaveGlobalID  ProfileTypeBytecodeFlag = 2
	ProfileTypeBytecodeFunctionArgument     ProfileTypeBytecodeFlag = 3
	ProfileTypeBytecodeFunctionReturnStatement ProfileTypeBytecodeFlag = 4
)
