package cmd

import (
	"fmt"

	"github.com/degree-analytics/ratatosk/internal/tracking"
)

func loadTrackingConfigForAccount(flags *RootFlags) (string, *tracking.Config, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return "", nil, err
	}

	cfg, err := tracking.LoadConfig(account)
	if err != nil {
		return "", nil, fmt.Errorf("load tracking config: %w", err)
	}

	return account, cfg, nil
}
