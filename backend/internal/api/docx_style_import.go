package api

func enhanceImportedDocxHTML(docxBytes []byte, htmlFragment string) string {
	plan, err := extractDocxImportStylePlan(docxBytes)
	if err != nil || plan.isEmpty() {
		return htmlFragment
	}
	enhanced, err := applyDocxImportStylePlanToHTML(htmlFragment, plan)
	if err != nil {
		return htmlFragment
	}
	return enhanced
}
