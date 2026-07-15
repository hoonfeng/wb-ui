$f = 'rendering\renderformcontrol.go'
$c = [System.IO.File]::ReadAllText($f)
$old = @'
	if textW > maxTextW {
		// Truncate rune by rune until it fits with "…"
		runes := []rune(displayText)
		runes = runes[:len(runes)-1]
	}
'@
$new = @'
	var runes []rune
	if textW > maxTextW {
		// Truncate rune by rune until it fits with "…"
		runes = []rune(displayText)
		if (len(runes) > 0) { runes = runes[:len(runes)-1] }
	}
'@
$c2 = $c.Replace($old, $new)
if ($c -ne $c2) { [System.IO.File]::WriteAllText($f, $c2); Write-Host OK } else { Write-Host 'FAIL: no match'; exit 1 }
