package notes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"

	"minivoip/store"
)

const (
	scribeURL   = "https://api.elevenlabs.io/v1/speech-to-text"
	scribeModel = "scribe_v2"
	// Long enough to send a long meeting's recording and have it written down.
	scribeTimeout = 30 * time.Minute
	// A transcript of more than a month's hours of notes is no transcript.
	maxScribeAnswer = 64 << 20
)

// Scribe is ElevenLabs' speech to text, with who said what. The language isn't
// set: the service hears it, so the transcript keeps the words as spoken, even
// when a caller speaks English to a French account.
type Scribe struct {
	Key string
	URL string // the real service unless a test sets it
}

type scribeAnswer struct {
	AudioSeconds *float64 `json:"audio_duration_secs"`
	Words        []struct {
		Text    string   `json:"text"`
		Type    string   `json:"type"` // word, spacing or audio_event
		Start   *float64 `json:"start"`
		End     *float64 `json:"end"`
		Speaker string   `json:"speaker_id"`
	} `json:"words"`
}

func (c *Scribe) Transcribe(ctx context.Context, file, contentType string) (Transcript, error) {
	f, err := os.Open(file)
	if err != nil {
		return Transcript{}, err
	}
	defer f.Close()

	// The recording is sent as it's read, not kept in memory.
	body, send := io.Pipe()
	form := multipart.NewWriter(send)
	go func() {
		send.CloseWithError(writeScribeForm(form, f, filepath.Base(file), contentType))
	}()

	url := c.URL
	if url == "" {
		url = scribeURL
	}
	ctx, cancel := context.WithTimeout(ctx, scribeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return Transcript{}, err
	}
	req.Header.Set("xi-api-key", c.Key)
	req.Header.Set("Content-Type", form.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		return Transcript{}, fmt.Errorf("scribe: %w", err)
	}
	defer resp.Body.Close()
	answer, err := io.ReadAll(io.LimitReader(resp.Body, maxScribeAnswer))
	if err != nil {
		return Transcript{}, fmt.Errorf("scribe: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Transcript{}, statusError("scribe", resp.StatusCode, answer)
	}
	var a scribeAnswer
	if err := json.Unmarshal(answer, &a); err != nil {
		return Transcript{}, fmt.Errorf("scribe: answer: %w", err)
	}
	return a.transcript(), nil
}

func writeScribeForm(form *multipart.Writer, f io.Reader, name, contentType string) error {
	for _, field := range [][2]string{{"model_id", scribeModel}, {"diarize", "true"}, {"timestamps_granularity", "word"}} {
		if err := form.WriteField(field[0], field[1]); err != nil {
			return err
		}
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, name))
	h.Set("Content-Type", contentType)
	part, err := form.CreatePart(h)
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, f); err != nil {
		return err
	}
	return form.Close()
}

// transcript puts the words together, a new line each time someone else speaks.
func (a scribeAnswer) transcript() Transcript {
	var t Transcript
	var text strings.Builder
	var last float64
	flush := func() {
		if len(t.Lines) > 0 {
			t.Lines[len(t.Lines)-1].Text = strings.TrimSpace(text.String())
		}
		text.Reset()
	}
	for _, w := range a.Words {
		if w.End != nil {
			last = max(last, *w.End)
		}
		switch w.Type {
		case "word":
			if len(t.Lines) == 0 || t.Lines[len(t.Lines)-1].Speaker != w.Speaker {
				flush()
				line := store.NoteLine{Speaker: w.Speaker}
				if w.Start != nil {
					line.Start = *w.Start
				}
				t.Lines = append(t.Lines, line)
			}
			text.WriteString(w.Text)
		case "spacing":
			text.WriteString(w.Text)
		}
	}
	flush()
	if a.AudioSeconds != nil {
		last = *a.AudioSeconds
	}
	t.Seconds = int64(math.Ceil(last))
	return t
}
