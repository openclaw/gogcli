package cmd

import "net/http"

func driveAllDrivesQueryError(r *http.Request, wantAllDrives bool, expectCorpora bool) string {
	q := r.URL.Query()
	if q.Get("supportsAllDrives") != "true" {
		return "missing supportsAllDrives=true"
	}
	if wantAllDrives {
		if expectCorpora {
			if q.Get("corpora") != "allDrives" {
				return "missing corpora=allDrives"
			}
		} else {
			if q.Get("corpora") != "" {
				return "unexpected corpora (expected none)"
			}
		}
		if q.Get("includeItemsFromAllDrives") != "true" {
			return "missing includeItemsFromAllDrives=true"
		}
		return ""
	}
	if got := q.Get("corpora"); got != "" {
		return "unexpected corpora (expected none)"
	}
	if q.Get("includeItemsFromAllDrives") != "false" {
		return "missing includeItemsFromAllDrives=false"
	}
	return ""
}
