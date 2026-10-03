// Package voice converts speech at the edge of the text conversation.
package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mu/internal/settings"
)

var client = &http.Client{Timeout: 45 * time.Second}

func Configured() bool { return settings.Get("GOOGLE_SPEECH_API_KEY") != "" }

func call(ctx context.Context, endpoint string, body, result any) error {
	key := settings.Get("GOOGLE_SPEECH_API_KEY")
	if key == "" {
		return errors.New("voice is not configured on this server")
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", key)
	rsp, err := client.Do(req)
	if err != nil {
		return errors.New("the speech provider could not be reached")
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("the speech provider refused the request (%d)", rsp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(rsp.Body, 8<<20)).Decode(result)
}

// Transcribe accepts at most thirty seconds of mono, signed little-endian PCM.
func Transcribe(ctx context.Context, pcm []byte, rate int, language string) (string, error) {
	if rate < 8000 || rate > 48000 || len(pcm) == 0 || len(pcm)%2 != 0 || len(pcm) > rate*2*30 {
		return "", errors.New("record up to 30 seconds of mono audio")
	}
	if language == "" {
		language = "en-GB"
	}
	if len(language) > 35 {
		return "", errors.New("invalid speech language")
	}
	var out struct {
		Results []struct {
			Alternatives []struct {
				Transcript string `json:"transcript"`
			} `json:"alternatives"`
		} `json:"results"`
	}
	err := call(ctx, "https://speech.googleapis.com/v1/speech:recognize", map[string]any{
		"config": map[string]any{"encoding": "LINEAR16", "sampleRateHertz": rate, "languageCode": language, "enableAutomaticPunctuation": true},
		"audio":  map[string]string{"content": base64.StdEncoding.EncodeToString(pcm)},
	}, &out)
	if err != nil {
		return "", err
	}
	var parts []string
	for _, result := range out.Results {
		if len(result.Alternatives) > 0 {
			parts = append(parts, result.Alternatives[0].Transcript)
		}
	}
	return strings.TrimSpace(strings.Join(parts, " ")), nil
}

func Speak(ctx context.Context, text string) ([]byte, error) {
	if strings.TrimSpace(text) == "" || len(text) > 4500 {
		return nil, errors.New("speech must contain between 1 and 4500 bytes of text")
	}
	language := settings.Get("VOICE_LANGUAGE")
	if language == "" {
		language = "en-GB"
	}
	v := map[string]string{"languageCode": language}
	if name := settings.Get("GOOGLE_SPEECH_VOICE"); name != "" {
		v["name"] = name
	}
	var out struct {
		Audio string `json:"audioContent"`
	}
	if err := call(ctx, "https://texttospeech.googleapis.com/v1/text:synthesize", map[string]any{"input": map[string]string{"text": text}, "voice": v, "audioConfig": map[string]string{"audioEncoding": "MP3"}}, &out); err != nil {
		return nil, err
	}
	audio, err := base64.StdEncoding.DecodeString(out.Audio)
	if err == nil && len(audio) == 0 {
		err = errors.New("the speech provider returned no audio")
	}
	return audio, err
}
