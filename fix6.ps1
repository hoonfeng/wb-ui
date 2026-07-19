$c = [System.IO.File]::ReadAllText('jsc\evaluator.go')
$c = $c.Replace('runtime.JSValueNumber(', 'runtime.NewJSValueNumber(')
$c = $c.Replace('runtime.JSValueString(', 'runtime.NewJSValueString(')
$c = $c.Replace('runtime.JSValueNaN', 'runtime.NewJSValueNumber(math.NaN())')
# 检查是否有缺失 math import
$hasMath = $c.Contains('"math"')
$hasRuntime := $c.Contains('"wb-ui/jsc/runtime"')
if (-not $hasMath) {
    $c = $c.Replace('"wb-ui/jsc/runtime"', '"math"`n`t"wb-ui/jsc/runtime"')
}
[System.IO.File]::WriteAllText('jsc\evaluator.go', $c)
Write-Host 'done2'
