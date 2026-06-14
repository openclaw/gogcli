package mailmime

import (
	"crypto/rand"
	"os"
	"time"
)

type (
	mailAttachment         = Attachment
	mailAttachmentMetadata = AttachmentMetadata
	mailOptions            = Options
)

type rfc822Config struct {
	allowMissingTo bool
	dateLocation   *time.Location
}

func testConfig() Config {
	return Config{
		DateLocation: time.Local,
		Now:          time.Now,
		Random:       rand.Reader,
		ReadFile:     os.ReadFile,
	}
}

func buildRFC822(opts mailOptions, cfg *rfc822Config) ([]byte, error) {
	config := testConfig()
	if cfg != nil {
		config.AllowMissingTo = cfg.allowMissingTo
		if cfg.dateLocation != nil {
			config.DateLocation = cfg.dateLocation
		}
	}

	return BuildRFC822(opts, config)
}

func prepareMailAttachments(attachments []mailAttachment) ([]mailAttachment, []mailAttachmentMetadata, error) {
	return PrepareAttachments(attachments, os.ReadFile)
}
