$c = [System.IO.File]::ReadAllText('jsc\evaluator.go')
$c = $c.Replace('runtime.JSValueNumber(', 'runtime.NewJSValueNumber(')
$c = $c.Replace('runtime.JSValueString(', 'runtime.NewJSValueString(')
$c = $c.Replace('runtime.JSValueNaN', 'runtime.NewJSValueNumber(math.NaN())')
[System.IO.File]::WriteAllText('jsc\evaluator.go', $c)
Write-Host 'done'
