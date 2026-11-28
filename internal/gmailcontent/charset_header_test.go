package gmailcontent

import "testing"

// The Content-Type header is written by whoever sent the message. Bytes before
// the charset parameter whose lower-case form has a different length (an
// invalid UTF-8 byte, U+0130, U+212A) must not change where the label is read
// from, and must not panic.
func TestDecodeBodyCharset_HeaderBytesBeforeLabel(t *testing.T) {
	for _, tc := range []struct {
		name        string
		contentType string
		in          string
		want        string
	}{
		{"invalid byte before the label", "text/plain; name=\xb6; charset=windows-1252;;", "\x93", "“"},
		{"dotted capital I before the label", "text/plain; name=İ; charset=windows-1252;;", "\x93", "“"},
		{"kelvin sign before the label", "text/plain; name=K; charset=windows-1252;;", "\x93", "“"},
		{"label cut short by the shift", "\xb6ChArset=", "0", "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(DecodeBodyCharset([]byte(tc.in), tc.contentType)); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func FuzzDecodeBodyCharset(f *testing.F) {
	f.Add([]byte("h\xe9llo"), "text/plain; charset=iso-8859-1")
	f.Add([]byte("\x1b$B$3$s\x1b(B"), "text/plain; charset=iso-2022-jp")
	f.Add([]byte("abc"), `text/html; charset="utf-16"`)
	f.Add([]byte("0"), "\xb6ChArset=")
	f.Fuzz(func(_ *testing.T, data []byte, contentType string) {
		_ = DecodeBodyCharset(data, contentType)
	})
}
