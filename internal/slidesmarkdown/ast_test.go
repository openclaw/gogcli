package slidesmarkdown

import "testing"

func TestBlockMarkerMethods(t *testing.T) {
	var _ Block = ParagraphBlock{}
	var _ Block = BulletsBlock{}
	var _ Block = CodeBlock{}
	var _ Block = HeadingBlock{}
	var _ Block = ColumnsBlock{}
	var _ Block = IconRowsBlock{}
	var _ Block = DiagramBlock{}

	var _ Inline = TextRun{}
	var _ Inline = IconRef{}
}
