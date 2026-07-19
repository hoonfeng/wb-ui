$c = [System.IO.File]::ReadAllText('jsc\evaluator.go')
$c = $c.Replace('runtime.JSValueNumber(', 'runtime.NewJSValueNumber(')
$c = $c.Replace('runtime.JSValueString(', 'runtime.NewJSValueString(')
$c = $c.Replace('runtime.JSValueNaN', 'runtime.NewJSValueNumber(math.NaN())')
# Add math import if missing
if ($c.Contains('"math"') -eq $false) {
    $c = $c.Replace('"wb-ui/jsc/runtime"', '"math"' + [Environment]::NewLine + [char]9 + '"wb-ui/jsc/runtime"')
}
[System.IO.File]::WriteAllText('jsc\evaluator.go', $c)
Write-Host 'done'
