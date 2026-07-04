package api

import (
	"archive/zip"
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestExtractDocxImportStylePlanCapturesDirectWordFormatting(t *testing.T) {
	docx := buildImportTestDocx(t, testImportDocumentXML())
	plan, err := extractDocxImportStylePlan(docx)
	if err != nil {
		t.Fatalf("extractDocxImportStylePlan returned error: %v", err)
	}
	if len(plan.Blocks) != 2 {
		t.Fatalf("expected 2 block styles, got %d: %+v", len(plan.Blocks), plan.Blocks)
	}
	if plan.Blocks[0].Text != "Centered Heading" || plan.Blocks[0].Align != "center" {
		t.Fatalf("unexpected heading block style: %+v", plan.Blocks[0])
	}
	if plan.Blocks[1].Align != "right" {
		t.Fatalf("unexpected paragraph alignment: %+v", plan.Blocks[1])
	}
	styledRun := plan.Blocks[1].Runs[len(plan.Blocks[1].Runs)-1]
	if styledRun.Text != "Styled span" ||
		styledRun.Color != "DC2626" ||
		styledRun.BackgroundColor != "FEF08A" ||
		styledRun.FontSizeHalfPoints != 36 {
		t.Fatalf("unexpected imported run style: %+v", styledRun)
	}
	if len(plan.TableCells) != 1 ||
		plan.TableCells[0].Text != "Cell B" ||
		plan.TableCells[0].Align != "center" ||
		plan.TableCells[0].BackgroundColor != "BFDBFE" {
		t.Fatalf("unexpected imported table cell style: %+v", plan.TableCells)
	}
	if len(plan.Images) != 1 || plan.Images[0].Align != "right" || plan.Images[0].WidthPercent != 50 {
		t.Fatalf("unexpected imported image style: %+v", plan.Images)
	}
}

func TestApplyDocxImportStylePlanToHTMLAddsEditorCompatibleStyles(t *testing.T) {
	plan := docxStylePlan{
		Blocks: []docxBlockStyle{
			{Text: "Centered Heading", Align: "center"},
			{
				Text:  "Plain Styled span",
				Align: "right",
				Runs: []docxRunStyle{
					{Text: "Plain"},
					{Text: "Styled span", Color: "DC2626", BackgroundColor: "FEF08A", FontSizeHalfPoints: 36},
				},
			},
		},
		TableCells: []docxTableCellStyle{{Text: "Cell B", Align: "center", BackgroundColor: "BFDBFE"}},
		Images:     []docxImageStyle{{Align: "right", WidthPercent: 50}},
	}
	html := `<h1 id="centered-heading">Centered Heading</h1>
<p>Plain Styled span</p>
<p><img src="media/image1.png" alt="demo" /></p>
<table><tbody><tr><td>Cell B</td></tr></tbody></table>`

	got, err := applyDocxImportStylePlanToHTML(html, plan)
	if err != nil {
		t.Fatalf("applyDocxImportStylePlanToHTML returned error: %v", err)
	}
	for _, want := range []string{
		`<h1 id="centered-heading" style="text-align:center">Centered Heading</h1>`,
		`<p style="text-align:right">Plain <span style="color:#DC2626;background-color:#FEF08A;font-size:24px">Styled span</span></p>`,
		`data-align="right"`,
		`data-width="50"`,
		`width:50%`,
		`<td style="text-align:center;background-color:#BFDBFE">Cell B</td>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("enhanced HTML missing %q:\n%s", want, got)
		}
	}
}

func TestDocxToHTMLPreservesDirectRichTextStylesWithPandoc(t *testing.T) {
	if _, err := exec.LookPath("pandoc"); err != nil {
		t.Skip("pandoc is not available")
	}
	docx := buildImportTestDocx(t, testImportDocumentXML())
	html, _, err := newPandocConverter().DocxToHTML(context.Background(), docx)
	if err != nil {
		t.Fatalf("DocxToHTML returned error: %v", err)
	}
	for _, want := range []string{
		`text-align:center`,
		`text-align:right`,
		`color:#DC2626`,
		`background-color:#FEF08A`,
		`font-size:24px`,
		`background-color:#BFDBFE`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("imported HTML missing %q:\n%s", want, html)
		}
	}
}

func buildImportTestDocx(t *testing.T, documentXML string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entries := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`,
		"word/_rels/document.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`,
		"word/document.xml": documentXML,
	}
	for name, content := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close docx: %v", err)
	}
	return buffer.Bytes()
}

func testImportDocumentXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
<w:body>
<w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:t>Centered Heading</w:t></w:r></w:p>
<w:p><w:pPr><w:jc w:val="right"/></w:pPr><w:r><w:t xml:space="preserve">Plain </w:t></w:r><w:r><w:rPr><w:color w:val="DC2626"/><w:sz w:val="36"/><w:shd w:val="clear" w:color="auto" w:fill="FEF08A"/></w:rPr><w:t>Styled span</w:t></w:r></w:p>
<w:p><w:pPr><w:jc w:val="right"/></w:pPr><w:r><w:drawing><wp:inline><wp:extent cx="2971800" cy="1485900"/><a:graphic><a:graphicData><pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:spPr><a:xfrm><a:ext cx="2971800" cy="1485900"/></a:xfrm></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>
<w:tbl><w:tr><w:tc><w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="BFDBFE"/></w:tcPr><w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:t>Cell B</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
<w:sectPr><w:pgSz w:w="12240" w:h="15840"/><w:pgMar w:left="1440" w:right="1440"/></w:sectPr>
</w:body></w:document>`
}
