package api

const defaultDocxBodyWidthEMU int64 = 9360 * 635

type docxStylePlan struct {
	Blocks     []docxBlockStyle
	TableCells []docxTableCellStyle
	Images     []docxImageStyle
}

type docxBlockStyle struct {
	Text  string
	Align string
	Runs  []docxRunStyle
}

type docxTableCellStyle struct {
	Text            string
	Align           string
	BackgroundColor string
}

type docxImageStyle struct {
	Align        string
	WidthPercent int
}

type docxRunStyle struct {
	Text               string
	Color              string
	BackgroundColor    string
	FontSizeHalfPoints int
}
