<#
.SYNOPSIS
  Builds the table of contents in the generated documentation through Word.

.DESCRIPTION
  docgen.py writes the document with an empty TOC field and asks Word to
  refresh fields on open. That is enough for a reader, but not for a file that
  is about to be mailed or converted: this updates the field up front so the
  contents list and its page numbers are already in the saved document.

  Two Word COM traps are avoided here, both learned the hard way:

    * $doc.Save() blocks forever on these templates. SaveAs2 does the same job
      in about a second.
    * SaveAs2 onto the path Word already has open fails silently, leaving the
      stale field values in place. So the refreshed document is written beside
      the original and then promoted over it.

.PARAMETER Path
  The .docx to refresh. Defaults to the generated documentation in docs/.
#>
[CmdletBinding()]
param(
    [string]$Path
)

$ErrorActionPreference = 'Stop'

if (-not $Path) {
    $docs = Resolve-Path (Join-Path $PSScriptRoot '..\..')
    $Path = Join-Path $docs 'mCollaborator Extensive Documentation.docx'
}
if (-not (Test-Path $Path)) { throw "not found: $Path" }
$Path = (Resolve-Path $Path).Path
$staged = [System.IO.Path]::ChangeExtension($Path, '.refreshed.docx')

Write-Host "==> refreshing $([System.IO.Path]::GetFileName($Path))"
$word = New-Object -ComObject Word.Application
$word.Visible = $false
try {
    $doc = $word.Documents.Open($Path, $false, $false)

    # Guarded individually: one stubborn field must not sink the whole run.
    try { $doc.TablesOfContents.Item(1).Update() } catch { Write-Warning "TOC: $_" }
    try { $doc.Fields.Update() | Out-Null } catch { Write-Warning "fields: $_" }

    if (Test-Path $staged) { Remove-Item $staged -Force }
    $doc.SaveAs2($staged)
    $doc.Close(0)
} finally {
    $word.Quit()
}

Move-Item $staged $Path -Force
Write-Host "    done: $Path"
