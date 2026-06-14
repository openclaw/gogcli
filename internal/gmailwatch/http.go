//nolint:tagliatelle // Google Pub/Sub and Gmail push payloads use their published camelCase schemas.
package gmailwatch

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var (
	ErrPushBodyTooLarge     = errors.New("push body too large")
	ErrMissingPushData      = errors.New("missing message.data")
	ErrInvalidPushHistoryID = errors.New("historyId must be string or number")
)

type PushEnvelope struct {
	Message struct {
		Data        string            `json:"data"`
		MessageID   string            `json:"messageId"`
		PublishTime string            `json:"publishTime"`
		Attributes  map[string]string `json:"attributes"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

type PushPayload struct {
	EmailAddress string `json:"emailAddress"`
	HistoryID    string `json:"historyId"`
	MessageID    string `json:"-"`
}

func (p *PushPayload) UnmarshalJSON(data []byte) error {
	var raw struct {
		EmailAddress string          `json:"emailAddress"`
		HistoryID    json.RawMessage `json:"historyId"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("decode push payload: %w", err)
	}

	p.EmailAddress = raw.EmailAddress
	if len(raw.HistoryID) == 0 {
		p.HistoryID = ""

		return nil
	}

	var asString string
	if err := json.Unmarshal(raw.HistoryID, &asString); err == nil {
		p.HistoryID = asString

		return nil
	}

	var asNumber json.Number
	if err := json.Unmarshal(raw.HistoryID, &asNumber); err == nil {
		if value := strings.TrimSpace(asNumber.String()); value != "" {
			p.HistoryID = value

			return nil
		}
	}

	return ErrInvalidPushHistoryID
}

type HTTPConfig struct {
	Path        string
	Account     string
	BodyLimit   int64
	HasHook     bool
	AllowNoHook bool
}

type HTTPHandler struct {
	Config    HTTPConfig
	Authorize func(*http.Request) bool
	Process   func(context.Context, Notification) (*ProcessedPayload, error)
	Now       func() time.Time
	Warnf     func(string, ...any)
}

func (h *HTTPHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if !PathMatches(h.Config.Path, request.URL.Path) {
		response.WriteHeader(http.StatusNotFound)

		return
	}

	if request.Method != http.MethodPost {
		response.Header().Set("Allow", http.MethodPost)
		response.WriteHeader(http.StatusMethodNotAllowed)

		return
	}

	if h.Authorize != nil && !h.Authorize(request) {
		response.WriteHeader(http.StatusUnauthorized)

		return
	}

	envelope, err := ParsePush(request, h.Config.BodyLimit)
	if err != nil {
		h.warnf("watch: invalid push payload: %v", err)
		response.WriteHeader(http.StatusBadRequest)

		return
	}

	payload, err := DecodePushPayload(envelope)
	if err != nil {
		h.warnf("watch: invalid push data: %v", err)
		response.WriteHeader(http.StatusBadRequest)

		return
	}

	if payload.EmailAddress != "" && !strings.EqualFold(payload.EmailAddress, h.Config.Account) {
		h.warnf("watch: ignoring push for %s", payload.EmailAddress)
		response.WriteHeader(http.StatusAccepted)

		return
	}

	if h.Process == nil {
		h.warnf("watch: handle push failed: missing processor")
		response.WriteHeader(http.StatusInternalServerError)

		return
	}

	processed, err := h.Process(request.Context(), Notification{
		HistoryID: payload.HistoryID,
		MessageID: payload.MessageID,
	})
	if err != nil {
		if errors.Is(err, ErrNoNewMessages) {
			response.WriteHeader(http.StatusAccepted)

			return
		}

		var rateErr *RateLimitError
		if errors.As(err, &rateErr) {
			if !rateErr.Until.IsZero() {
				response.Header().Set("Retry-After", RetryAfterSeconds(h.currentTime(), rateErr.Until))
			}

			h.warnf("watch: Gmail rate limit circuit open: %v", err)
			response.WriteHeader(http.StatusServiceUnavailable)

			return
		}

		h.warnf("watch: handle push failed: %v", err)
		response.WriteHeader(http.StatusInternalServerError)

		return
	}

	if processed == nil || processed.Payload == nil {
		response.WriteHeader(http.StatusAccepted)

		return
	}

	if !h.Config.HasHook {
		if h.Config.AllowNoHook {
			_ = json.NewEncoder(response).Encode(processed.Payload)

			return
		}

		response.WriteHeader(http.StatusAccepted)

		return
	}

	response.WriteHeader(http.StatusOK)
}

func ParsePush(request *http.Request, bodyLimit int64) (*PushEnvelope, error) {
	defer request.Body.Close()

	data, err := io.ReadAll(io.LimitReader(request.Body, bodyLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read push body: %w", err)
	}

	if int64(len(data)) > bodyLimit {
		return nil, ErrPushBodyTooLarge
	}

	var envelope PushEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("decode push envelope: %w", err)
	}

	if envelope.Message.Data == "" {
		return nil, ErrMissingPushData
	}

	return &envelope, nil
}

func DecodePushPayload(envelope *PushEnvelope) (PushPayload, error) {
	decoded, err := base64.StdEncoding.DecodeString(envelope.Message.Data)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(envelope.Message.Data)
		if err != nil {
			return PushPayload{}, fmt.Errorf("decode push data: %w", err)
		}
	}

	var payload PushPayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return PushPayload{}, fmt.Errorf("decode Gmail push payload: %w", err)
	}
	payload.MessageID = strings.TrimSpace(envelope.Message.MessageID)

	return payload, nil
}

func PathMatches(expected, actual string) bool {
	if expected == actual {
		return true
	}

	if strings.HasSuffix(expected, "/") {
		return strings.HasPrefix(actual, expected)
	}

	return strings.HasPrefix(actual, expected+"/")
}

func RetryAfterSeconds(now, until time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}

	seconds := int64(until.Sub(now).Seconds())
	if seconds < 1 {
		seconds = 1
	}

	return strconv.FormatInt(seconds, 10)
}

func (h *HTTPHandler) currentTime() time.Time {
	if h.Now != nil {
		return h.Now()
	}

	return time.Now()
}

func (h *HTTPHandler) warnf(format string, args ...any) {
	if h.Warnf != nil {
		h.Warnf(format, args...)
	}
}
