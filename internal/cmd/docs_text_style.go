package cmd

import "google.golang.org/api/docs/v1"

const docsTextStyleResetFields = "bold,italic,underline,strikethrough,smallCaps,baselineOffset,foregroundColor,backgroundColor,fontSize,weightedFontFamily,link"

func resetDocsTextStyleRequest(startIdx, endIdx int64, tabID string) *docs.Request {
	return &docs.Request{
		UpdateTextStyle: &docs.UpdateTextStyleRequest{
			Range: &docs.Range{
				StartIndex: startIdx,
				EndIndex:   endIdx,
				TabId:      tabID,
			},
			TextStyle: &docs.TextStyle{},
			Fields:    docsTextStyleResetFields,
		},
	}
}
