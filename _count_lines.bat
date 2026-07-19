@echo off
for %%f in (
  jsc\parser\parser.go
  jsc\parser\nodes.go
  jsc\parser\lexer.go
  jsc\parser\ast_builder.go
  jsc\bytecompiler\bytecode_generator.go
  jsc\interpreter\interpreter.go
  jsc\bytecode\code_block.go
  jsc\runtime\jsobject.go
  jsc\runtime\jsvalue.go
  jsc\runtime\promise.go
  jsc\runtime\proxy_object.go
) do (
  for /f "usebackq tokens=3" %%i in (`find /c /v "" %%f`) do echo %%f: %%i lines
)
