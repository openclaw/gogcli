package cmd

import (
	"google.golang.org/api/meet/v2"

	"github.com/openclaw/gogcli/internal/ui"
)

// MeetSpaceConfigFlags are the space settings shared by meet create and update.
// Unset flags are not sent, so the account's Meet defaults (or the current value) apply.
type MeetSpaceConfigFlags struct {
	Recording         *bool `name:"recording" negatable:"" help:"Auto-record when someone allowed to record joins; omit to use account defaults"`
	Transcription     *bool `name:"transcription" negatable:"" help:"Auto-transcribe when someone allowed to transcribe joins; omit to use account defaults"`
	SmartNotes        *bool `name:"smart-notes" negatable:"" help:"Auto-generate Gemini smart notes; omit to use account defaults"`
	AttendanceReport  *bool `name:"attendance-report" negatable:"" help:"Generate an attendance report; omit to use account defaults"`
	Moderation        *bool `name:"moderation" negatable:"" help:"Enable host management; omit to use account defaults"`
	RestrictChat      *bool `name:"restrict-chat" negatable:"" help:"Only hosts can chat (enables --moderation)"`
	RestrictPresent   *bool `name:"restrict-present" negatable:"" help:"Only hosts can share their screen (enables --moderation)"`
	RestrictReactions *bool `name:"restrict-reactions" negatable:"" help:"Only hosts can send reactions (enables --moderation)"`
	JoinAsViewer      *bool `name:"join-as-viewer" negatable:"" help:"Participants join as viewers by default (enables --moderation)"`
}

func (f MeetSpaceConfigFlags) hasRestriction() bool {
	return f.RestrictChat != nil || f.RestrictPresent != nil || f.RestrictReactions != nil || f.JoinAsViewer != nil
}

// apply writes the set flags into cfg and returns the matching update mask paths.
func (f MeetSpaceConfigFlags) apply(cfg *meet.SpaceConfig) ([]string, error) {
	moderation := f.Moderation
	if f.hasRestriction() {
		if moderation != nil && !*moderation {
			return nil, usage("--no-moderation cannot be combined with the --[no-]restrict-* or --[no-]join-as-viewer flags")
		}
		// The API rejects restriction changes, in either direction, unless moderation is ON.
		on := true
		moderation = &on
	}

	var mask []string

	add := func(value *bool, field string, apply func(on bool)) {
		if value == nil {
			return
		}

		apply(*value)

		mask = append(mask, "config."+field)
	}
	add(f.Recording, "artifactConfig.recordingConfig.autoRecordingGeneration", func(on bool) {
		meetArtifactConfig(cfg).RecordingConfig = &meet.RecordingConfig{AutoRecordingGeneration: meetOnOff(on)}
	})
	add(f.Transcription, "artifactConfig.transcriptionConfig.autoTranscriptionGeneration", func(on bool) {
		meetArtifactConfig(cfg).TranscriptionConfig = &meet.TranscriptionConfig{AutoTranscriptionGeneration: meetOnOff(on)}
	})
	add(f.SmartNotes, "artifactConfig.smartNotesConfig.autoSmartNotesGeneration", func(on bool) {
		meetArtifactConfig(cfg).SmartNotesConfig = &meet.SmartNotesConfig{AutoSmartNotesGeneration: meetOnOff(on)}
	})
	add(f.AttendanceReport, "attendanceReportGenerationType", func(on bool) {
		cfg.AttendanceReportGenerationType = "DO_NOT_GENERATE"
		if on {
			cfg.AttendanceReportGenerationType = "GENERATE_REPORT"
		}
	})
	add(moderation, "moderation", func(on bool) { cfg.Moderation = meetOnOff(on) })
	add(f.RestrictChat, "moderationRestrictions.chatRestriction", func(on bool) {
		meetModerationRestrictions(cfg).ChatRestriction = meetRestriction(on)
	})
	add(f.RestrictPresent, "moderationRestrictions.presentRestriction", func(on bool) {
		meetModerationRestrictions(cfg).PresentRestriction = meetRestriction(on)
	})
	add(f.RestrictReactions, "moderationRestrictions.reactionRestriction", func(on bool) {
		meetModerationRestrictions(cfg).ReactionRestriction = meetRestriction(on)
	})
	add(f.JoinAsViewer, "moderationRestrictions.defaultJoinAsViewerType", func(on bool) {
		meetModerationRestrictions(cfg).DefaultJoinAsViewerType = meetOnOff(on)
	})

	return mask, nil
}

func meetArtifactConfig(cfg *meet.SpaceConfig) *meet.ArtifactConfig {
	if cfg.ArtifactConfig == nil {
		cfg.ArtifactConfig = &meet.ArtifactConfig{}
	}

	return cfg.ArtifactConfig
}

func meetModerationRestrictions(cfg *meet.SpaceConfig) *meet.ModerationRestrictions {
	if cfg.ModerationRestrictions == nil {
		cfg.ModerationRestrictions = &meet.ModerationRestrictions{}
	}

	return cfg.ModerationRestrictions
}

func meetOnOff(on bool) string {
	if on {
		return "ON"
	}

	return "OFF"
}

func meetRestriction(hostsOnly bool) string {
	if hostsOnly {
		return "HOSTS_ONLY"
	}

	return "NO_RESTRICTION"
}

// printMeetSpaceConfig prints settings using the on/off sense of the matching flags.
func printMeetSpaceConfig(u *ui.UI, cfg *meet.SpaceConfig) {
	line := func(key, value, onValue, offValue string) {
		switch value {
		case onValue:
			u.Out().Linef("%s\ton", key)
		case offValue:
			u.Out().Linef("%s\toff", key)
		}
	}

	if a := cfg.ArtifactConfig; a != nil {
		if a.RecordingConfig != nil {
			line("recording", a.RecordingConfig.AutoRecordingGeneration, "ON", "OFF")
		}

		if a.TranscriptionConfig != nil {
			line("transcription", a.TranscriptionConfig.AutoTranscriptionGeneration, "ON", "OFF")
		}

		if a.SmartNotesConfig != nil {
			line("smart_notes", a.SmartNotesConfig.AutoSmartNotesGeneration, "ON", "OFF")
		}
	}

	line("attendance_report", cfg.AttendanceReportGenerationType, "GENERATE_REPORT", "DO_NOT_GENERATE")
	line("moderation", cfg.Moderation, "ON", "OFF")

	if r := cfg.ModerationRestrictions; r != nil {
		line("restrict_chat", r.ChatRestriction, "HOSTS_ONLY", "NO_RESTRICTION")
		line("restrict_present", r.PresentRestriction, "HOSTS_ONLY", "NO_RESTRICTION")
		line("restrict_reactions", r.ReactionRestriction, "HOSTS_ONLY", "NO_RESTRICTION")
		line("join_as_viewer", r.DefaultJoinAsViewerType, "ON", "OFF")
	}
}
