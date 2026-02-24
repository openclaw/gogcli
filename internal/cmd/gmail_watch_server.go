package cmd

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/idtoken"
)

var errNoNewMessages = errors.New("no new messages")

const (
	gmailWatchFormatMetadata  = "metadata"
	gmailWatchStatusHTTPError = "http_error"
)

type gmailWatchServer struct {
	cfg             gmailWatchServeConfig
	store           *gmailWatchStore
	validator       *idtoken.Validator
	newService      func(context.Context, string) (*gmail.Service, error)
	hookClient      *http.Client
	excludeLabelIDs map[string]struct{}
	logf            func(string, ...any)
	warnf           func(string, ...any)
}

func (s *gmailWatchServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !pathMatches(s.cfg.Path, r.URL.Path) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if ok := s.authorize(r); !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	push, err := parsePubSubPush(r)
	if err != nil {
		s.warnf("watch: invalid push payload: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	payload, err := decodeGmailPushPayload(push)
	if err != nil {
		s.warnf("watch: invalid push data: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if payload.EmailAddress != "" && !strings.EqualFold(payload.EmailAddress, s.cfg.Account) {
		s.warnf("watch: ignoring push for %s", payload.EmailAddress)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	result, err := s.handlePush(r.Context(), payload)
	if err != nil {
		if errors.Is(err, errNoNewMessages) {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		s.warnf("watch: handle push failed: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if result == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	if s.cfg.HookURL == "" {
		if s.cfg.AllowNoHook {
			_ = json.NewEncoder(w).Encode(result)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}

	if err := s.sendHook(r.Context(), result); err != nil {
		s.warnf("watch: hook failed: %v", err)
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *gmailWatchServer) authorize(r *http.Request) bool {
	if s.cfg.VerifyOIDC {
		bearer := bearerToken(r)
		if bearer != "" {
			if ok, err := verifyOIDCToken(r.Context(), s.validator, bearer, s.oidcAudience(r), s.cfg.OIDCEmail); ok {
				return true
			} else if err != nil {
				s.warnf("watch: oidc verify failed: %v", err)
			}
		}
		if s.cfg.SharedToken != "" {
			return sharedTokenMatches(r, s.cfg.SharedToken)
		}
		return false
	}
	if s.cfg.SharedToken == "" {
		return true
	}
	return sharedTokenMatches(r, s.cfg.SharedToken)
}

func (s *gmailWatchServer) oidcAudience(r *http.Request) string {
	if s.cfg.OIDCAudience != "" {
		return s.cfg.OIDCAudience
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if xf := r.Header.Get("X-Forwarded-Proto"); xf != "" {
		parts := strings.Split(xf, ",")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			scheme = strings.TrimSpace(parts[0])
		}
	}
	host := r.Host
	if xf := r.Header.Get("X-Forwarded-Host"); xf != "" {
		parts := strings.Split(xf, ",")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			host = strings.TrimSpace(parts[0])
		}
	}
	return fmt.Sprintf("%s://%s%s", scheme, host, r.URL.Path)
}

func (s *gmailWatchServer) handlePush(ctx context.Context, payload gmailPushPayload) (*gmailHookPayload, error) {
	store := s.store
	if payload.MessageID != "" {
		state := store.Get()
		if state.LastPushMessageID == payload.MessageID {
			s.logf("watch: ignoring duplicate push %s", payload.MessageID)
			return nil, errNoNewMessages
		}
	}
	startID, err := store.StartHistoryID(payload.HistoryID)
	if err != nil {
		return nil, err
	}
	if startID == 0 {
		if payload.HistoryID != "" {
			state := store.Get()
			stale, staleErr := isStaleHistoryID(state.HistoryID, payload.HistoryID)
			if staleErr != nil {
				s.warnf("watch: history id compare failed: %v", staleErr)
			} else if stale {
				s.logf("watch: ignoring stale push historyId=%s (stored=%s)", payload.HistoryID, state.HistoryID)
			}
		}
		return nil, errNoNewMessages
	}

	svc, err := s.newService(ctx, s.cfg.Account)
	if err != nil {
		return nil, err
	}

	historyCall := svc.Users.History.List("me").StartHistoryId(startID).MaxResults(s.cfg.HistoryMax)
	if len(s.cfg.HistoryTypes) > 0 {
		historyCall.HistoryTypes(s.cfg.HistoryTypes...)
	}

	historyResp, err := historyCall.Do()
	if err != nil {
		if isStaleHistoryError(err) {
			return s.resyncHistory(ctx, svc, payload.HistoryID, payload.MessageID)
		}
		return nil, err
	}

	nextHistoryID := payload.HistoryID
	if historyResp != nil && historyResp.HistoryId != 0 {
		nextHistoryID = formatHistoryID(historyResp.HistoryId)
	}
	if len(s.cfg.HistoryTypes) > 0 && (historyResp == nil || len(historyResp.History) == 0) {
		if updateErr := store.Update(func(state *gmailWatchState) error {
			return updateStateAfterHistory(state, nextHistoryID, payload.MessageID)
		}); updateErr != nil {
			s.warnf("watch: failed to update state: %v", updateErr)
		}
		return nil, errNoNewMessages
	}

	historyIDs := collectHistoryMessageIDs(historyResp)
	msgs, excluded, err := s.fetchMessages(ctx, svc, historyIDs.FetchIDs)
	if err != nil {
		return nil, err
	}
	if err := store.Update(func(state *gmailWatchState) error {
		return updateStateAfterHistory(state, nextHistoryID, payload.MessageID)
	}); err != nil {
		s.warnf("watch: failed to update state: %v", err)
	}

	if excluded > 0 && len(msgs) == 0 {
		if s.cfg.VerboseOutput {
			s.logf("watch: skipping hook; all messages excluded")
		}
		return nil, errNoNewMessages
	}

	return &gmailHookPayload{
		Source:            "gmail",
		Account:           s.cfg.Account,
		HistoryID:         nextHistoryID,
		Messages:          msgs,
		DeletedMessageIDs: historyIDs.DeletedIDs,
	}, nil
}

func (s *gmailWatchServer) resyncHistory(ctx context.Context, svc *gmail.Service, historyID string, messageID string) (*gmailHookPayload, error) {
	list, err := svc.Users.Messages.List("me").MaxResults(s.cfg.ResyncMax).Do()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(list.Messages))
	for _, m := range list.Messages {
		if m != nil && m.Id != "" {
			ids = append(ids, m.Id)
		}
	}
	msgs, excluded, err := s.fetchMessages(ctx, svc, ids)
	if err != nil {
		return nil, err
	}

	if err := s.store.Update(func(state *gmailWatchState) error {
		return updateStateAfterHistory(state, historyID, messageID)
	}); err != nil {
		s.warnf("watch: failed to update state after resync: %v", err)
	}

	if excluded > 0 && len(msgs) == 0 {
		if s.cfg.VerboseOutput {
			s.logf("watch: skipping hook; all messages excluded")
		}
		return nil, errNoNewMessages
	}

	return &gmailHookPayload{
		Source:    "gmail",
		Account:   s.cfg.Account,
		HistoryID: historyID,
		Messages:  msgs,
	}, nil
}

func (s *gmailWatchServer) fetchMessages(ctx context.Context, svc *gmail.Service, ids []string) ([]gmailHookMessage, int, error) {
	messages := make([]gmailHookMessage, 0, len(ids))
	excluded := 0
	format := gmailWatchFormatMetadata
	if s.cfg.IncludeBody {
		format = gmailFormatFull
	}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			continue
		}
		msg, err := svc.Users.Messages.Get("me", id).
			Format(format).
			MetadataHeaders("From", "To", "Subject", "Date").
			Context(ctx).
			Do()
		if err != nil {
			if isNotFoundAPIError(err) {
				continue
			}
			return nil, excluded, err
		}
		if msg == nil {
			continue
		}
		if s.isExcludedLabel(msg.LabelIds) {
			excluded++
			if s.cfg.VerboseOutput {
				s.logf("watch: excluded message %s labels=%v", msg.Id, msg.LabelIds)
			}
			continue
		}
		item := gmailHookMessage{
			ID:       msg.Id,
			ThreadID: msg.ThreadId,
			From:     headerValue(msg.Payload, "From"),
			To:       headerValue(msg.Payload, "To"),
			Subject:  headerValue(msg.Payload, "Subject"),
			Date:     formatGmailDateInLocation(headerValue(msg.Payload, "Date"), s.cfg.DateLocation),
			Snippet:  msg.Snippet,
			Labels:   msg.LabelIds,
		}
		if s.cfg.IncludeBody {
			body := bestBodyText(msg.Payload)
			item.Body, item.BodyTruncated = truncateUTF8Bytes(body, s.cfg.MaxBodyBytes)
		}
		messages = append(messages, item)
	}
	return messages, excluded, nil
}

func (s *gmailWatchServer) isExcludedLabel(labelIDs []string) bool {
	if len(labelIDs) == 0 || len(s.excludeLabelIDs) == 0 {
		return false
	}
	for _, id := range labelIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		if _, ok := s.excludeLabelIDs[trimmed]; ok {
			return true
		}
	}
	return false
}

func (s *gmailWatchServer) sendHook(ctx context.Context, payload *gmailHookPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.HookURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.cfg.HookToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.HookToken)
	}
	resp, err := s.hookClient.Do(req)
	if err != nil {
		_ = s.store.Update(func(state *gmailWatchState) error {
			state.LastDeliveryStatus = "error"
			state.LastDeliveryAtMs = time.Now().UnixMilli()
			state.LastDeliveryStatusNote = err.Error()
			return nil
		})
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = s.store.Update(func(state *gmailWatchState) error {
			state.LastDeliveryStatus = gmailWatchStatusHTTPError
			state.LastDeliveryAtMs = time.Now().UnixMilli()
			state.LastDeliveryStatusNote = fmt.Sprintf("status %d", resp.StatusCode)
			return nil
		})
		return fmt.Errorf("hook status %d", resp.StatusCode)
	}
	_ = s.store.Update(func(state *gmailWatchState) error {
		state.LastDeliveryStatus = "ok"
		state.LastDeliveryAtMs = time.Now().UnixMilli()
		state.LastDeliveryStatusNote = ""
		return nil
	})
	return nil
}

func parsePubSubPush(r *http.Request) (*pubsubPushEnvelope, error) {
	defer r.Body.Close()
	limit := int64(defaultPushBodyLimitBytes)
	data, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("push body too large")
	}
	var envelope pubsubPushEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	if envelope.Message.Data == "" {
		return nil, errors.New("missing message.data")
	}
	return &envelope, nil
}

func decodeGmailPushPayload(envelope *pubsubPushEnvelope) (gmailPushPayload, error) {
	decoded, err := base64.StdEncoding.DecodeString(envelope.Message.Data)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(envelope.Message.Data)
		if err != nil {
			return gmailPushPayload{}, err
		}
	}
	var payload gmailPushPayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return gmailPushPayload{}, err
	}
	payload.MessageID = strings.TrimSpace(envelope.Message.MessageID)
	return payload, nil
}

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func sharedTokenMatches(r *http.Request, expected string) bool {
	if expected == "" {
		return false
	}
	token := r.Header.Get("x-gog-token")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

func verifyOIDCToken(ctx context.Context, validator *idtoken.Validator, token, audience, expectedEmail string) (bool, error) {
	if validator == nil {
		return false, errors.New("no OIDC validator")
	}
	payload, err := validator.Validate(ctx, token, audience)
	if err != nil {
		return false, err
	}
	if expectedEmail == "" {
		return true, nil
	}
	email, _ := payload.Claims["email"].(string)
	if !strings.EqualFold(email, expectedEmail) {
		return false, fmt.Errorf("oidc email mismatch: %s", email)
	}
	return true, nil
}

func pathMatches(expected, actual string) bool {
	if expected == actual {
		return true
	}
	if strings.HasSuffix(expected, "/") {
		return strings.HasPrefix(actual, expected)
	}
	return strings.HasPrefix(actual, expected+"/")
}

// updateStateAfterHistory updates the stored state with the new history ID and push message ID.
// This is a common operation after processing history, whether messages were found or not.
func updateStateAfterHistory(state *gmailWatchState, historyID, pushMessageID string) error {
	shouldUpdate, err := shouldUpdateHistoryID(state.HistoryID, historyID)
	if err != nil {
		return err
	}
	if shouldUpdate {
		state.HistoryID = historyID
	}
	if pushMessageID != "" {
		state.LastPushMessageID = pushMessageID
	}
	state.UpdatedAtMs = time.Now().UnixMilli()
	return nil
}

func isStaleHistoryError(err error) bool {
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		if gerr.Code == http.StatusBadRequest || gerr.Code == http.StatusNotFound {
			msg := strings.ToLower(gerr.Message)
			if strings.Contains(msg, "history") {
				return true
			}
			for _, item := range gerr.Errors {
				if strings.Contains(strings.ToLower(item.Message), "history") {
					return true
				}
				if gerr.Code == http.StatusNotFound && strings.EqualFold(strings.TrimSpace(item.Reason), "notfound") {
					return true
				}
			}
			if gerr.Code == http.StatusNotFound && strings.Contains(msg, "not found") {
				return true
			}
		}
	}
	return strings.Contains(strings.ToLower(err.Error()), "history")
}

func isNotFoundAPIError(err error) bool {
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		return gerr.Code == http.StatusNotFound
	}
	return false
}

// historyMessageIDs holds the result of collecting message IDs from history.
// FetchIDs contains messages that should be fetched (added, label changes, etc.).
// DeletedIDs contains messages that were deleted and cannot be fetched.
type historyMessageIDs struct {
	FetchIDs   []string
	DeletedIDs []string
}

func collectHistoryMessageIDs(resp *gmail.ListHistoryResponse) historyMessageIDs {
	if resp == nil || len(resp.History) == 0 {
		return historyMessageIDs{}
	}
	fetchIdx := make(map[string]int)
	seenDeleted := make(map[string]struct{})
	var result historyMessageIDs

	addFetch := func(id string) {
		if id == "" {
			return
		}
		if _, ok := fetchIdx[id]; ok {
			return
		}
		// If already marked as deleted, don't add to fetch list
		if _, ok := seenDeleted[id]; ok {
			return
		}
		fetchIdx[id] = len(result.FetchIDs)
		result.FetchIDs = append(result.FetchIDs, id)
	}

	addDeleted := func(id string) {
		if id == "" {
			return
		}
		if _, ok := seenDeleted[id]; ok {
			return
		}
		// Mark as removed from fetch list if previously added.
		// A final compaction keeps stable order while avoiding O(n) delete per tombstone.
		if i, ok := fetchIdx[id]; ok {
			delete(fetchIdx, id)
			result.FetchIDs[i] = ""
		}
		seenDeleted[id] = struct{}{}
		result.DeletedIDs = append(result.DeletedIDs, id)
	}

	messageID := func(msg *gmail.Message) string {
		if msg == nil {
			return ""
		}
		return strings.TrimSpace(msg.Id)
	}

	for _, h := range resp.History {
		if h == nil {
			continue
		}
		for _, added := range h.MessagesAdded {
			if added == nil {
				continue
			}
			addFetch(messageID(added.Message))
		}
		for _, deleted := range h.MessagesDeleted {
			if deleted == nil {
				continue
			}
			addDeleted(messageID(deleted.Message))
		}
		for _, added := range h.LabelsAdded {
			if added == nil {
				continue
			}
			addFetch(messageID(added.Message))
		}
		for _, removed := range h.LabelsRemoved {
			if removed == nil {
				continue
			}
			addFetch(messageID(removed.Message))
		}
		for _, msg := range h.Messages {
			addFetch(messageID(msg))
		}
	}
	if len(fetchIdx) != len(result.FetchIDs) {
		compacted := result.FetchIDs[:0]
		for _, id := range result.FetchIDs {
			if id != "" {
				compacted = append(compacted, id)
			}
		}
		result.FetchIDs = compacted
	}
	return result
}
