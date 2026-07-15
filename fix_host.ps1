$content = [System.IO.File]::ReadAllText('F:\syproject\wb-ui\app\host.go')

$old = "`t`t`t`t`tif !h.hysteresisMet {`r`n" +
       "`t`t`t`t`t`tdx := cssX - h.mouseDownX`r`n" +
       "`t`t`t`t`t`tdy := cssY - h.mouseDownY`r`n" +
       "`t`t`t`t`t`tif dx > -3 && dx < 3 && dy > -3 && dy < 3 {`r`n" +
       "`t`t`t`t`t`t`tcontinue // not yet dragging`r`n" +
       "`t`t`t`t`t`t}`r`n" +
       "`t`t`t`t`t// Update the cursor-move selection end point.`r`n" +
       "`t`t`t`t`t`tif dx > -3 && dx < 3 && dy > -3 && dy < 3 {`r`n" +
       "`t`t`t`t`t`t`tcontinue // not yet dragging`r`n" +
       "`t`t`t`t`t`t}`r`n" +
       "`t`t`t`t`t// Update the cursor-move selection end point."

$new = "`t`t`t`t`tif !h.hysteresisMet {`r`n" +
       "`t`t`t`t`t`tdx := cssX - h.mouseDownX`r`n" +
       "`t`t`t`t`t`tdy := cssY - h.mouseDownY`r`n" +
       "`t`t`t`t`t`tif dx > -3 && dx < 3 && dy > -3 && dy < 3 {`r`n" +
       "`t`t`t`t`t`t`tcontinue // not yet dragging`r`n" +
       "`t`t`t`t`t`t}`r`n" +
       "`t`t`t`t`t}`r`n" +
       "`t`t`t`t`t// Update the cursor-move selection end point."

if ($content.Contains($old)) {
    $content = $content.Replace($old, $new)
    [System.IO.File]::WriteAllText('F:\syproject\wb-ui\app\host.go', $content)
    Write-Host "OK"
} else {
    Write-Host "NO MATCH"
    exit 1
}
