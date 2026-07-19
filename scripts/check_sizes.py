import os

files = [
    'parser/ASTBuilder.h', 'parser/Parser.h', 'parser/SyntaxChecker.h', 
    'parser/ParserArena.h', 'parser/ParserModes.h', 'parser/ParserTokens.h',
    'parser/ParserError.h', 'parser/ResultType.h', 'parser/SourceCode.h',
    'parser/SourceProvider.h', 'parser/VariableEnvironment.h', 'parser/Lexer.h',
    'parser/NodeConstructors.h', 'parser/ParserFunctionInfo.h',
    'bytecompiler/BytecodeGenerator.h', 'bytecompiler/BytecodeGeneratorBase.h',
    'bytecompiler/RegisterID.h', 'bytecompiler/Label.h', 'bytecompiler/LabelScope.h',
    'bytecompiler/ProfileTypeBytecodeFlag.h', 'bytecompiler/StaticPropertyAnalysis.h',
    'bytecompiler/StaticPropertyAnalyzer.h'
]
base = 'F:/syproject/ref/WebKit/Source/JavaScriptCore/'
for f in files:
    path = os.path.join(base, f)
    if os.path.exists(path):
        with open(path, 'rb') as fh:
            lines = sum(1 for _ in fh)
        size = os.path.getsize(path)
        print(f'{f}: {lines}行, {size/1024:.0f}KB')
    else:
        print(f'{f}: 不存在')
