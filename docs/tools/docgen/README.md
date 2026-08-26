# docgen — the application documentation generator

Builds `docs/mCollaborator Extensive Documentation.docx` from the Cyberteq
document template, so the documentation can be revised in text and rebuilt
rather than hand-edited in Word.

## Running it

```powershell
pip install python-docx
python docgen.py
.\refresh-toc.ps1     # builds the table of contents through Word
```

`docgen.py` writes the document and sets it to refresh fields on open, so a
reader gets a correct contents list either way. `refresh-toc.ps1` does that
refresh up front, which is what to run before the file is sent to anyone or
converted to PDF.

Both take optional arguments if you need to work on a copy:

```powershell
python docgen.py "<template.docx>" "<output.docx>"
.\refresh-toc.ps1 -Path "<output.docx>"
```

## How it works

| File | Contents |
|---|---|
| `docgen.py` | The builder: strips the template's own content after the table of contents and renders the block list in its place |
| `content_a.py` | Sections 1–9 — overview through the web application |
| `content_b.py` | Sections 10–17 — the report pipeline through the glossary |
| `refresh-toc.ps1` | Word COM pass that builds the TOC field |

The template is `docs/mCollaborator Documentation.docx`. Only its **styling** is
used — the cover, the letterhead header and footer, the numbered `Heading2` /
`Heading3` styles, `Table Grid`, the bullet list and the Wavehaus fonts. Its own
text is discarded, so that file can be edited freely as long as those styles
survive.

Content is a list of `(kind, value)` blocks:

```python
("h2",        "Security Model")
("h3",        "Authentication")
("p",         "Text with **bold** and `code` spans.")
("bullets",   ["first", "second"])
("code",      "go build -o mCollaborator.exe .")
("table",     (["Header"], [["cell"]], [2.0]))   # headers, rows, column widths
("pagebreak",)
```

Column widths are in inches and should total 6.2 or less; the text column of an
A4 page with the template's margins is 6.27 in.

## Things that will bite

- **Word locks the template.** With `mCollaborator Documentation.docx` open in
  Word, `python-docx` reports "package not found". Close it, or pass a copy as
  the first argument.
- **`$doc.Save()` hangs** under Word COM on these documents. `refresh-toc.ps1`
  uses `SaveAs2` to a staged file and promotes it, because `SaveAs2` onto the
  path Word already has open fails silently and leaves stale field values.
- **The bullet list is the template's `numId` 44.** A `List Paragraph` style
  alone renders without a bullet glyph; the numbering reference has to be set on
  each paragraph, which `Builder.bullet` does.
- **Tables must be fixed-layout.** Column widths set on an autofit table are
  ignored by Word, so the width is written onto the grid and every cell.
