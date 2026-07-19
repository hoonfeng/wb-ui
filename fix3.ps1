$c = [System.IO.File]::ReadAllText('jsc\parser\parser.go')
$c = $c.Replace('NUMBER', 'INTEGER')
$c = $c.Replace('DELETE', 'DELETETOKEN')
[System.IO.File]::WriteAllText('jsc\parser\parser.go', $c)
Write-Host 'done'
