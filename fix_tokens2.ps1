$file = "jsc\parser\parser.go"
$content = [System.IO.File]::ReadAllText((Resolve-Path $file))
$replacements = @{
    "tokOF" = "OF"
    "tokOR" = "OR"
    "tokAND" = "AND"
    "tokBITOR" = "BITOR"
    "tokBITXOR" = "BITXOR"
    "tokBITAND" = "BITAND"
    "tokNE" = "NE"
    "tokSTREQ" = "STREQ"
    "tokSTRNE" = "STRNEQ"
    "tokLT" = "LT"
    "tokGT" = "GT"
    "tokLE" = "LE"
    "tokGE" = "GE"
    "tokINSTANCEOF" = "INSTANCEOF"
    "tokIN" = "INTOKEN"
    "tokLSHIFT" = "LSHIFT"
    "tokRSHIFT" = "RSHIFT"
    "tokURSHIFT" = "URSHIFT"
    "tokPLUS" = "PLUS"
    "tokMINUS" = "MINUS"
    "tokDIVIDE" = "DIVIDE"
    "tokMOD" = "MOD"
    "tokEXP" = "POW"
    "tokTILD" = "TILDE"
    "tokRROTEQ" = "URSHIFTEQUAL"
    "tokGET" = "IDENT"
    "tokSET" = "IDENT"
    "tokSTATIC" = "IDENT"
    "tokTHISTOKEN" = "THISTOKEN"
}
foreach ($k in $replacements.Keys) {
    $content = $content.Replace($k, $replacements[$k])
}
# Remove the tokOF from isIdentifierOrKeyword since OF is checked separately
$content = $content.Replace("OF != IDENT", "OF != IDENT")
[System.IO.File]::WriteAllText((Resolve-Path $file), $content)
Write-Host "Done round 2"
