package api

import (
	"archive/zip"
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestExtractDocxStylePlanCapturesSupportedRichText(t *testing.T) {
	plan, err := extractDocxStylePlan(`<h1 style="text-align:center">Centered Heading</h1>
<p style="text-align:right">Plain <span style="color:#dc2626;font-size:24px;background-color:#fef08a">Styled span</span></p>
<img data-align="right" data-width="50" src=".assets/demo.png">
<table><tr><td style="background-color:#bfdbfe;text-align:center">Cell B</td></tr></table>`)
	if err != nil {
		t.Fatalf("extractDocxStylePlan returned error: %v", err)
	}
	if len(plan.Blocks) != 2 {
		t.Fatalf("expected 2 block styles, got %d", len(plan.Blocks))
	}
	if plan.Blocks[0].Align != "center" || plan.Blocks[0].Text != "Centered Heading" {
		t.Fatalf("unexpected heading style: %+v", plan.Blocks[0])
	}
	if plan.Blocks[1].Align != "right" {
		t.Fatalf("expected paragraph right alignment, got %+v", plan.Blocks[1])
	}
	styledRun := plan.Blocks[1].Runs[len(plan.Blocks[1].Runs)-1]
	if styledRun.Text != "Styled span" ||
		styledRun.Color != "DC2626" ||
		styledRun.BackgroundColor != "FEF08A" ||
		styledRun.FontSizeHalfPoints != 36 {
		t.Fatalf("unexpected styled run: %+v", styledRun)
	}
	if len(plan.Images) != 1 || plan.Images[0].Align != "right" || plan.Images[0].WidthPercent != 50 {
		t.Fatalf("unexpected image style plan: %+v", plan.Images)
	}
	if len(plan.TableCells) != 1 ||
		plan.TableCells[0].Align != "center" ||
		plan.TableCells[0].BackgroundColor != "BFDBFE" {
		t.Fatalf("unexpected table cell style: %+v", plan.TableCells)
	}
}

func TestEnhanceDocxRichTextStylesPatchesDocumentXML(t *testing.T) {
	docx := buildTestDocx(t, testDocumentXML())
	html := `<h1 style="text-align:center">Centered Heading</h1>
<p style="text-align:right">Plain <span style="color:#dc2626;font-size:24px;background-color:#fef08a">Styled span</span></p>
<img data-align="right" data-width="50" src=".assets/demo.png">
<table><tr><td style="background-color:#bfdbfe;text-align:center">Cell B</td></tr></table>`

	got := enhanceDocxRichTextStyles(docx, html)
	documentXML := readDocxDocumentXML(t, got)

	for _, want := range []string{
		`<w:jc w:val="center" />`,
		`<w:jc w:val="right" />`,
		`<w:color w:val="DC2626" />`,
		`<w:sz w:val="36" />`,
		`<w:szCs w:val="36" />`,
		`<w:shd w:val="clear" w:color="auto" w:fill="FEF08A" />`,
		`<w:shd w:val="clear" w:color="auto" w:fill="BFDBFE" />`,
		`cx="2971800"`,
		`cy="1485900"`,
	} {
		if !strings.Contains(documentXML, want) {
			t.Fatalf("patched document XML missing %q:\n%s", want, documentXML)
		}
	}
}

func TestHTMLToDocxPreservesRichTextStylesWithPandoc(t *testing.T) {
	if _, err := exec.LookPath("pandoc"); err != nil {
		t.Skip("pandoc is not available")
	}
	files := newAPITestWorkspace(t)
	html := `<h1 style="text-align:center">Centered Heading</h1>
<p>Plain <span style="color:#dc2626;font-size:24px;background-color:#fef08a">Styled span</span></p>`

	docxBytes, err := newPandocConverter().HTMLToDocx(context.Background(), files, "filename-check.html", html)
	if err != nil {
		t.Fatalf("HTMLToDocx returned error: %v", err)
	}
	documentXML := readDocxDocumentXML(t, docxBytes)
	for _, want := range []string{
		`Centered Heading`,
		`Styled span`,
		`<w:jc w:val="center" />`,
		`<w:color w:val="DC2626" />`,
		`<w:sz w:val="36" />`,
		`<w:shd w:val="clear" w:color="auto" w:fill="FEF08A" />`,
	} {
		if !strings.Contains(documentXML, want) {
			t.Fatalf("exported docx XML missing %q:\n%s", want, documentXML)
		}
	}
	if strings.Contains(documentXML, "filename-check") {
		t.Fatalf("exported docx should not include source filename in body:\n%s", documentXML)
	}
}

func testDocumentXML() string {
	return `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><w:body>
<w:p><w:pPr><w:pStyle w:val="Heading1" /></w:pPr><w:r><w:t xml:space="preserve">Centered Heading</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="FirstParagraph" /></w:pPr><w:r><w:t xml:space="preserve">Plain</w:t></w:r><w:r><w:t xml:space="preserve">Styled span</w:t></w:r></w:p>
<w:p><w:pPr /><w:r><w:drawing><wp:inline><wp:extent cx="1000" cy="500" /><a:graphic><a:graphicData><pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:spPr><a:xfrm><a:ext cx="1000" cy="500" /></a:xfrm></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>
<w:tbl><w:tr><w:tc><w:tcPr /><w:p><w:pPr /><w:r><w:t xml:space="preserve">Cell B</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
<w:sectPr><w:pgSz w:w="12240" w:h="15840" /><w:pgMar w:left="1440" w:right="1440" /></w:sectPr>
</w:body></w:document>`
}

func buildTestDocx(t *testing.T, documentXML string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	doc, err := writer.Create("word/document.xml")
	if err != nil {
		t.Fatalf("create document.xml: %v", err)
	}
	if _, err := doc.Write([]byte(documentXML)); err != nil {
		t.Fatalf("write document.xml: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close docx: %v", err)
	}
	return buffer.Bytes()
}

func readDocxDocumentXML(t *testing.T, docx []byte) string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		t.Fatalf("open docx zip: %v", err)
	}
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		raw, err := readZipFile(file)
		if err != nil {
			t.Fatalf("read document.xml: %v", err)
		}
		return string(raw)
	}
	t.Fatalf("docx missing word/document.xml")
	return ""
}
