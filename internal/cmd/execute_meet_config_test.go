package cmd

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

type meetCapturedRequest struct {
	mask   string
	config map[string]any
}

// newMeetCaptureService answers get/create/patch and records the create or patch body.
func newMeetCaptureService(t *testing.T, captured *meetCapturedRequest) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/spaces/abc-defg-hij" && r.Method == http.MethodGet:
		case (r.URL.Path == "/v2/spaces" && r.Method == http.MethodPost) ||
			(r.URL.Path == "/v2/spaces/abc123" && r.Method == http.MethodPatch):
			var body struct {
				Config map[string]any `json:"config"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			captured.mask = r.URL.Query().Get("updateMask")
			captured.config = body.Config
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(meetSpaceResponse())
	}
}

func runMeetCapture(t *testing.T, args ...string) meetCapturedRequest {
	t.Helper()
	var captured meetCapturedRequest
	svc := newTestMeetService(t, newMeetCaptureService(t, &captured))
	result := executeWithMeetTestService(t, append([]string{"--json", "--account", "a@b.com", "meet"}, args...), svc)
	if result.err != nil {
		t.Fatalf("Execute %v: %v", args, result.err)
	}
	return captured
}

func TestExecute_MeetCreate_SendsSpaceConfig(t *testing.T) {
	got := runMeetCapture(t, "create",
		"--recording", "--no-transcription", "--smart-notes", "--attendance-report", "--restrict-chat")

	want := map[string]any{
		"accessType":       "TRUSTED",
		"entryPointAccess": "ALL",
		"artifactConfig": map[string]any{
			"recordingConfig":     map[string]any{"autoRecordingGeneration": "ON"},
			"transcriptionConfig": map[string]any{"autoTranscriptionGeneration": "OFF"},
			"smartNotesConfig":    map[string]any{"autoSmartNotesGeneration": "ON"},
		},
		"attendanceReportGenerationType": "GENERATE_REPORT",
		"moderation":                     "ON",
		"moderationRestrictions":         map[string]any{"chatRestriction": "HOSTS_ONLY"},
	}
	if !reflect.DeepEqual(got.config, want) {
		t.Fatalf("config =\n%#v\nwant\n%#v", got.config, want)
	}
}

func TestExecute_MeetCreate_UnsetFlagsUseAccountDefaults(t *testing.T) {
	got := runMeetCapture(t, "create")

	want := map[string]any{"accessType": "TRUSTED", "entryPointAccess": "ALL"}
	if !reflect.DeepEqual(got.config, want) {
		t.Fatalf("config = %#v, want %#v", got.config, want)
	}
}

func TestExecute_MeetUpdate_SpaceConfigFlags(t *testing.T) {
	tests := []struct {
		flag   string
		mask   string
		config map[string]any
	}{
		{
			"--recording", "config.artifactConfig.recordingConfig.autoRecordingGeneration",
			map[string]any{"artifactConfig": map[string]any{"recordingConfig": map[string]any{"autoRecordingGeneration": "ON"}}},
		},
		{
			"--no-recording", "config.artifactConfig.recordingConfig.autoRecordingGeneration",
			map[string]any{"artifactConfig": map[string]any{"recordingConfig": map[string]any{"autoRecordingGeneration": "OFF"}}},
		},
		{
			"--transcription", "config.artifactConfig.transcriptionConfig.autoTranscriptionGeneration",
			map[string]any{"artifactConfig": map[string]any{"transcriptionConfig": map[string]any{"autoTranscriptionGeneration": "ON"}}},
		},
		{
			"--no-transcription", "config.artifactConfig.transcriptionConfig.autoTranscriptionGeneration",
			map[string]any{"artifactConfig": map[string]any{"transcriptionConfig": map[string]any{"autoTranscriptionGeneration": "OFF"}}},
		},
		{
			"--smart-notes", "config.artifactConfig.smartNotesConfig.autoSmartNotesGeneration",
			map[string]any{"artifactConfig": map[string]any{"smartNotesConfig": map[string]any{"autoSmartNotesGeneration": "ON"}}},
		},
		{
			"--no-smart-notes", "config.artifactConfig.smartNotesConfig.autoSmartNotesGeneration",
			map[string]any{"artifactConfig": map[string]any{"smartNotesConfig": map[string]any{"autoSmartNotesGeneration": "OFF"}}},
		},
		{
			"--attendance-report", "config.attendanceReportGenerationType",
			map[string]any{"attendanceReportGenerationType": "GENERATE_REPORT"},
		},
		{
			"--no-attendance-report", "config.attendanceReportGenerationType",
			map[string]any{"attendanceReportGenerationType": "DO_NOT_GENERATE"},
		},
		{"--moderation", "config.moderation", map[string]any{"moderation": "ON"}},
		{"--no-moderation", "config.moderation", map[string]any{"moderation": "OFF"}},
		{
			"--restrict-chat", "config.moderation,config.moderationRestrictions.chatRestriction",
			map[string]any{"moderation": "ON", "moderationRestrictions": map[string]any{"chatRestriction": "HOSTS_ONLY"}},
		},
		{
			"--no-restrict-chat", "config.moderation,config.moderationRestrictions.chatRestriction",
			map[string]any{"moderation": "ON", "moderationRestrictions": map[string]any{"chatRestriction": "NO_RESTRICTION"}},
		},
		{
			"--restrict-present", "config.moderation,config.moderationRestrictions.presentRestriction",
			map[string]any{"moderation": "ON", "moderationRestrictions": map[string]any{"presentRestriction": "HOSTS_ONLY"}},
		},
		{
			"--no-restrict-present", "config.moderation,config.moderationRestrictions.presentRestriction",
			map[string]any{"moderation": "ON", "moderationRestrictions": map[string]any{"presentRestriction": "NO_RESTRICTION"}},
		},
		{
			"--restrict-reactions", "config.moderation,config.moderationRestrictions.reactionRestriction",
			map[string]any{"moderation": "ON", "moderationRestrictions": map[string]any{"reactionRestriction": "HOSTS_ONLY"}},
		},
		{
			"--no-restrict-reactions", "config.moderation,config.moderationRestrictions.reactionRestriction",
			map[string]any{"moderation": "ON", "moderationRestrictions": map[string]any{"reactionRestriction": "NO_RESTRICTION"}},
		},
		{
			"--join-as-viewer", "config.moderation,config.moderationRestrictions.defaultJoinAsViewerType",
			map[string]any{"moderation": "ON", "moderationRestrictions": map[string]any{"defaultJoinAsViewerType": "ON"}},
		},
		{
			"--no-join-as-viewer", "config.moderation,config.moderationRestrictions.defaultJoinAsViewerType",
			map[string]any{"moderation": "ON", "moderationRestrictions": map[string]any{"defaultJoinAsViewerType": "OFF"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.flag, func(t *testing.T) {
			got := runMeetCapture(t, "update", "abc-defg-hij", tc.flag)
			if got.mask != tc.mask {
				t.Fatalf("updateMask = %q, want %q", got.mask, tc.mask)
			}
			if !reflect.DeepEqual(got.config, tc.config) {
				t.Fatalf("config = %#v, want %#v", got.config, tc.config)
			}
		})
	}
}

func TestExecute_MeetUpdate_CombinesAccessAndSpaceConfigMask(t *testing.T) {
	got := runMeetCapture(t, "update", "abc-defg-hij", "--access", "open", "--no-recording", "--no-restrict-chat")

	wantMask := "config.accessType,config.artifactConfig.recordingConfig.autoRecordingGeneration,config.moderation,config.moderationRestrictions.chatRestriction"
	if got.mask != wantMask {
		t.Fatalf("updateMask = %q, want %q", got.mask, wantMask)
	}
}

func TestExecute_MeetSpaceConfig_NoModerationConflictsWithRestriction(t *testing.T) {
	for _, args := range [][]string{
		{"create", "--no-moderation", "--restrict-chat"},
		{"update", "abc-defg-hij", "--no-moderation", "--join-as-viewer"},
		{"create", "--no-moderation", "--no-restrict-chat"},
	} {
		result := executeWithMeetTestOperations(t,
			append([]string{"--json", "--account", "a@b.com", "meet"}, args...),
			unexpectedMeetTestService(t, "should fail before calling the API"),
			nil,
		)
		if ExitCode(result.err) != 2 || !strings.Contains(result.err.Error(), "--no-moderation cannot be combined") {
			t.Fatalf("%v: err = %v", args, result.err)
		}
	}
}

func TestExecute_MeetUpdate_RequiresSetting(t *testing.T) {
	result := executeWithMeetTestOperations(t,
		[]string{"--json", "--account", "a@b.com", "meet", "update", "abc-defg-hij"},
		unexpectedMeetTestService(t, "should fail before calling the API"),
		nil,
	)
	if ExitCode(result.err) != 2 || !strings.Contains(result.err.Error(), "at least one setting is required") {
		t.Fatalf("err = %v", result.err)
	}
}

func TestExecute_MeetUpdate_DryRunShowsSpaceConfigMask(t *testing.T) {
	result := executeWithMeetTestOperations(t,
		[]string{"--json", "--dry-run", "--account", "a@b.com", "meet", "update", "abc-defg-hij", "--recording", "--restrict-present"},
		unexpectedMeetTestService(t, "should not call API in dry-run mode"),
		nil,
	)
	if result.err != nil && ExitCode(result.err) != 0 {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed struct {
		Request struct {
			UpdateMask string `json:"update_mask"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}

	want := "config.artifactConfig.recordingConfig.autoRecordingGeneration,config.moderation,config.moderationRestrictions.presentRestriction"
	if parsed.Request.UpdateMask != want {
		t.Fatalf("update_mask = %q, want %q\nout=%s", parsed.Request.UpdateMask, want, result.stdout)
	}
}

func TestExecute_MeetGet_TextShowsSpaceConfig(t *testing.T) {
	svc := newTestMeetService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.URL.Path == "/v2/spaces/abc-defg-hij" && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		response := meetSpaceResponse()
		response["config"] = map[string]any{
			"accessType": "TRUSTED",
			"artifactConfig": map[string]any{
				"recordingConfig":     map[string]any{"autoRecordingGeneration": "ON"},
				"transcriptionConfig": map[string]any{"autoTranscriptionGeneration": "OFF"},
				"smartNotesConfig":    map[string]any{"autoSmartNotesGeneration": "ON"},
			},
			"attendanceReportGenerationType": "DO_NOT_GENERATE",
			"moderation":                     "ON",
			"moderationRestrictions": map[string]any{
				"chatRestriction":         "HOSTS_ONLY",
				"presentRestriction":      "NO_RESTRICTION",
				"reactionRestriction":     "NO_RESTRICTION",
				"defaultJoinAsViewerType": "OFF",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))

	result := executeWithMeetTestService(t, []string{"--plain", "--account", "a@b.com", "meet", "get", "abc-defg-hij"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	want := strings.Join([]string{
		"meeting_code\tabc-defg-hij",
		"meeting_uri\thttps://meet.google.com/abc-defg-hij",
		"access\ttrusted",
		"recording\ton",
		"transcription\toff",
		"smart_notes\ton",
		"attendance_report\toff",
		"moderation\ton",
		"restrict_chat\ton",
		"restrict_present\toff",
		"restrict_reactions\toff",
		"join_as_viewer\toff",
	}, "\n") + "\n"
	if result.stdout != want {
		t.Fatalf("stdout =\n%s\nwant\n%s", result.stdout, want)
	}
}
