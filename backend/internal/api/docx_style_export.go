package api

func enhanceDocxRichTextStyles(docxBytes []byte, htmlFragment string) []byte {
	plan, err := extractDocxStylePlan(htmlFragment)
	if err != nil || plan.isEmpty() {
		return docxBytes
	}
	enhanced, ok := rewriteDocxDocumentXML(docxBytes, func(documentXML string) string {
		return applyDocxStylePlan(documentXML, plan)
	})
	if !ok {
		return docxBytes
	}
	return enhanced
}

func (plan docxStylePlan) isEmpty() bool {
	return len(plan.Blocks) == 0 && len(plan.TableCells) == 0 && len(plan.Images) == 0
}
