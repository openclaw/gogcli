package cmd

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	defaultWatchPath             = "/gmail-pubsub"
	defaultWatchPort             = 8788
	defaultHookMaxBytes          = 20000
	defaultHistoryMaxResults     = 100
	defaultHistoryResyncMax      = 10
	defaultPushBodyLimitBytes    = 1024 * 1024
	defaultHookRequestTimeoutSec = 10
)

type gmailWatchHook struct {
	URL         string `json:"url"`
	Token       string `json:"token,omitempty"`
	IncludeBody bool   `json:"includeBody,omitempty"`
	MaxBytes    int    `json:"maxBytes,omitempty"`
}

type gmailWatchState struct {
	Account                string          `json:"account"`
	Topic                  string          `json:"topic"`
	Labels                 []string        `json:"labels,omitempty"`
	HistoryID              string          `json:"historyId"`
	ExpirationMs           int64           `json:"expirationMs,omitempty"`
	ProviderExpirationMs   int64           `json:"providerExpirationMs,omitempty"`
	RenewAfterMs           int64           `json:"renewAfterMs,omitempty"`
	UpdatedAtMs            int64           `json:"updatedAtMs,omitempty"`
	Hook                   *gmailWatchHook `json:"hook,omitempty"`
	LastDeliveryStatus     string          `json:"lastDeliveryStatus,omitempty"`
	LastDeliveryAtMs       int64           `json:"lastDeliveryAtMs,omitempty"`
	LastDeliveryStatusNote string          `json:"lastDeliveryStatusNote,omitempty"`
	LastPushMessageID      string          `json:"lastPushMessageId,omitempty"`
}

type gmailWatchServeConfig struct {
	Account       string
	Bind          string
	Port          int
	Path          string
	VerifyOIDC    bool
	OIDCEmail     string
	OIDCAudience  string
	SharedToken   string
	HookURL       string
	HookToken     string
	IncludeBody   bool
	MaxBodyBytes  int
	ExcludeLabels []string
	HistoryMax    int64
	ResyncMax     int64
	HookTimeout   time.Duration
	DateLocation  *time.Location
	PersistHook   bool
	AllowNoHook   bool
	VerboseOutput bool
}

type pubsubPushEnvelope struct {
	Message struct {
		Data        string            `json:"data"`
		MessageID   string            `json:"messageId"`
		PublishTime string            `json:"publishTime"`
		Attributes  map[string]string `json:"attributes"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

type gmailPushPayload struct {
	EmailAddress string `json:"emailAddress"`
	HistoryID    string `json:"historyId"`
	MessageID    string `json:"-"`
}

func (p *gmailPushPayload) UnmarshalJSON(data []byte) error {
	var raw struct {
		EmailAddress string          `json:"emailAddress"`
		HistoryID    json.RawMessage `json:"historyId"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
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
		if v := strings.TrimSpace(asNumber.String()); v != "" {
			p.HistoryID = v
			return nil
		}
	}
	return usage("historyId must be string or number")
}

type gmailHookMessage struct {
	ID            string   `json:"id"`
	ThreadID      string   `json:"threadId"`
	From          string   `json:"from,omitempty"`
	To            string   `json:"to,omitempty"`
	Subject       string   `json:"subject,omitempty"`
	Date          string   `json:"date,omitempty"`
	Snippet       string   `json:"snippet,omitempty"`
	Body          string   `json:"body,omitempty"`
	BodyTruncated bool     `json:"bodyTruncated,omitempty"`
	Labels        []string `json:"labels,omitempty"`
}

type gmailHookPayload struct {
	Source    string             `json:"source"`
	Account   string             `json:"account"`
	HistoryID string             `json:"historyId"`
	Messages  []gmailHookMessage `json:"messages"`
}
