package cmd

import (
	"context"
	"fmt"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/ui"
)

// One maximal page is enough to count all but the broadest result sets, and
// 500 is the ceiling Gmail's list endpoints accept.
const gmailCountProbePageSize = 500

// gmailMatchCount is how large a result set really is.
//
// Exact means the probe reached the end of the set, so Value is the total.
// Otherwise the probe filled its page with more behind it and Value is a lower
// bound — reported as such rather than rounded into a total nobody can trust.
type gmailMatchCount struct {
	Value int64
	Exact bool
}

// apply writes the count into a JSON payload under the name that matches its
// certainty, so a consumer never has to guess whether a number is a total.
func (c gmailMatchCount) apply(payload map[string]any) {
	if c.Exact {
		payload["totalMatches"] = c.Value
		return
	}
	payload["totalMatchesAtLeast"] = c.Value
}

// Deliberately NOT Gmail's resultSizeEstimate, which is the obvious source and
// saturates: measured against a live mailbox it returned exactly 201 for every
// non-empty query — from:freshbooks.com (21 real matches), from:housecallpro.com
// (6), from:thumbtack.com newer_than:30d (3) — and 0 for a query with none. It
// is a has-results boolean wearing a number's clothes, and it does not vary
// with maxResults either. Emitting it would let a caller report "3 of ~201"
// when the truth is 3 of 6. See openclaw/gogcli#983.
//
// Counting bare ids costs one extra list call and returns a response of ids
// alone, and it is exact whenever the set fits a single page — the common case,
// and always the case for the narrow queries where a wrong count does the most
// damage.
func countGmailThreadMatches(ctx context.Context, svc *gmail.Service, query string) (gmailMatchCount, error) {
	opts := newGmailSearchRequestOptions(query, gmailCountProbePageSize, "")
	resp, err := applyGmailThreadListOptions(svc.Users.Threads.List("me"), opts).
		Fields("threads/id,nextPageToken").
		Context(ctx).
		Do()
	if err != nil {
		return gmailMatchCount{}, err
	}
	return gmailMatchCount{Value: int64(len(resp.Threads)), Exact: resp.NextPageToken == ""}, nil
}

func countGmailMessageMatches(ctx context.Context, svc *gmail.Service, query string) (gmailMatchCount, error) {
	opts := newGmailSearchRequestOptions(query, gmailCountProbePageSize, "")
	resp, err := applyGmailMessageListOptions(svc.Users.Messages.List("me"), opts).
		Fields("messages/id,nextPageToken").
		Context(ctx).
		Do()
	if err != nil {
		return gmailMatchCount{}, err
	}
	return gmailMatchCount{Value: int64(len(resp.Messages)), Exact: resp.NextPageToken == ""}, nil
}

// The human-facing form of the same fact, on stderr so the table on stdout
// stays parseable. Says outright when the page is not the whole set: a caller
// reading only the visible rows is exactly how a partial result gets reported
// as an absence.
func printGmailMatchCount(u *ui.UI, shown int, count gmailMatchCount) {
	if count.Exact {
		if int64(shown) < count.Value {
			u.Err().Println(fmt.Sprintf("Showing %d of %d matches.", shown, count.Value))
			return
		}
		u.Err().Println(fmt.Sprintf("%d matches.", count.Value))
		return
	}
	u.Err().Println(fmt.Sprintf("Showing %d of at least %d matches.", shown, count.Value))
}

// resolveGmailMatchCount decides how --count is answered for one search, and
// whether it can be answered at all. Three cases, and only one of them is worth
// an extra request:
//
//	--results-only  The count is an envelope field, and --results-only exists
//	                to drop the envelope and emit the bare result array — so
//	                the number would be computed and then thrown away. Say so
//	                on stderr instead of spending a request in silence.
//	--all           When the walk started at the beginning, the items in hand
//	                ARE the whole set. A walk started with --page contains only
//	                the suffix, so it still needs a whole-query probe.
//	otherwise       Probe.
//
// The count is always for the WHOLE query, never the remainder after a
// --page cursor: "how many match this" is the number that stops a caller
// concluding an absence, and keeping it page-independent means it does not
// drift while paging through a set.
func resolveGmailMatchCount(
	u *ui.UI,
	resultsOnly, all bool,
	page string,
	shown int,
	probe func() (gmailMatchCount, error),
) (gmailMatchCount, bool, error) {
	switch {
	case resultsOnly:
		u.Err().Println("--count has no effect with --results-only: the count is an envelope field, and --results-only emits only the result array. Skipping the extra request.")
		return gmailMatchCount{}, false, nil
	case all && page == "":
		return gmailMatchCount{Value: int64(shown), Exact: true}, true, nil
	default:
		count, err := probe()
		if err != nil {
			return gmailMatchCount{}, false, err
		}
		return count, true, nil
	}
}
