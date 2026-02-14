package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/timeparse"
	"github.com/steipete/gogcli/internal/tracking"
	"github.com/steipete/gogcli/internal/ui"
)

const trackingUnknown = "unknown"

type GmailTrackOpensCmd struct {
	TrackingID string `arg:"" optional:"" help:"Tracking ID from send command"`
	To         string `name:"to" help:"Filter by recipient email"`
	Since      string `name:"since" help:"Filter by time (e.g., '24h', '2024-01-01')"`
}

func (c *GmailTrackOpensCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	_, cfg, err := loadTrackingConfigForAccount(flags)
	if err != nil {
		return err
	}
	if !cfg.IsConfigured() {
		return fmt.Errorf("tracking not configured; run 'gog gmail track setup' first")
	}

	// Query by tracking ID
	if c.TrackingID != "" {
		return c.queryByTrackingID(ctx, cfg, u)
	}

	// Query via admin endpoint
	return c.queryAdmin(ctx, cfg, u)
}

func (c *GmailTrackOpensCmd) queryByTrackingID(ctx context.Context, cfg *tracking.Config, u *ui.UI) error {
	reqURL := fmt.Sprintf("%s/q/%s", cfg.WorkerURL, c.TrackingID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("query tracker: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("tracker returned %d: %s", resp.StatusCode, body)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		var anyJSON any
		if err := json.Unmarshal(body, &anyJSON); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		return outfmt.WriteJSON(ctx, os.Stdout, anyJSON)
	}

	var result struct {
		TrackingID     string `json:"tracking_id"`
		Recipient      string `json:"recipient"`
		SentAt         string `json:"sent_at"`
		TotalOpens     int    `json:"total_opens"`
		HumanOpens     int    `json:"human_opens"`
		FirstHumanOpen *struct {
			At       string `json:"at"`
			Location *struct {
				City    string `json:"city"`
				Region  string `json:"region"`
				Country string `json:"country"`
			} `json:"location"`
		} `json:"first_human_open"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	u.Out().Printf("tracking_id\t%s", result.TrackingID)
	u.Out().Printf("recipient\t%s", result.Recipient)
	u.Out().Printf("sent_at\t%s", result.SentAt)
	u.Out().Printf("opens_total\t%d", result.TotalOpens)
	u.Out().Printf("opens_human\t%d", result.HumanOpens)

	if result.FirstHumanOpen != nil {
		u.Out().Printf("first_human_open\t%s", result.FirstHumanOpen.At)

		loc := trackingUnknown
		if result.FirstHumanOpen.Location != nil && result.FirstHumanOpen.Location.City != "" {
			loc = fmt.Sprintf("%s, %s", result.FirstHumanOpen.Location.City, result.FirstHumanOpen.Location.Region)
		}
		u.Out().Printf("first_human_open_location\t%s", loc)
	}

	return nil
}

func (c *GmailTrackOpensCmd) queryAdmin(ctx context.Context, cfg *tracking.Config, u *ui.UI) error {
	if strings.TrimSpace(cfg.AdminKey) == "" {
		return fmt.Errorf("tracking admin key not configured; run 'gog gmail track setup' again")
	}

	reqURL, _ := url.Parse(cfg.WorkerURL + "/opens")
	q := reqURL.Query()
	if c.To != "" {
		q.Set("recipient", c.To)
	}
	if c.Since != "" {
		since, err := parseTrackingSince(c.Since)
		if err != nil {
			return err
		}
		q.Set("since", since)
	}
	reqURL.RawQuery = q.Encode()

	req, _ := http.NewRequestWithContext(ctx, "GET", reqURL.String(), nil)
	req.Header.Set("Authorization", "Bearer "+cfg.AdminKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("query tracker: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return fmt.Errorf("unauthorized: admin key may be incorrect")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("tracker returned %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Opens []struct {
			TrackingID  string `json:"tracking_id"`
			Recipient   string `json:"recipient"`
			SubjectHash string `json:"subject_hash"`
			SentAt      string `json:"sent_at"`
			OpenedAt    string `json:"opened_at"`
			IsBot       bool   `json:"is_bot"`
			Location    *struct {
				City    string `json:"city"`
				Region  string `json:"region"`
				Country string `json:"country"`
			} `json:"location"`
		} `json:"opens"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, result)
	}

	if len(result.Opens) == 0 {
		u.Out().Printf("opens\t0")
		return nil
	}

	for _, o := range result.Opens {
		loc := trackingUnknown
		if o.Location != nil && o.Location.City != "" {
			loc = fmt.Sprintf("%s, %s", o.Location.City, o.Location.Region)
		}
		u.Out().Printf("%s\t%s\t%s\t%t\t%s\t%s", o.TrackingID, o.Recipient, o.OpenedAt, o.IsBot, o.SubjectHash, loc)
	}

	return nil
}

func parseTrackingSince(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", usage("empty --since")
	}

	parsed, err := timeparse.ParseSince(s, time.Now(), time.Local)
	if err != nil {
		return "", usagef("invalid --since %q (use duration like 24h, date YYYY-MM-DD, or RFC3339)", s)
	}
	if parsed.UseRFC3339Nano {
		return parsed.Time.Format(time.RFC3339Nano), nil
	}
	return parsed.Time.Format(time.RFC3339), nil
}
