package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/settings"
)

// ImageModelID is the Atlas Cloud text-to-image model Mu generates with.
// Atlas model ids are vendor/model/task (e.g. "google/nano-banana/text-to-image");
// the "/text-to-image" suffix is required. Override with the IMAGE_MODEL setting.
const ImageModelID = "google/nano-banana-2-lite/text-to-image"

// atlasImageBase is Atlas Cloud's async image API host.
const atlasImageBase = "https://api.atlascloud.ai"

var imageHTTPClient = &http.Client{Timeout: 30 * time.Second}

// imageModel returns the configured image model id.
func imageModel() string {
	if v := strings.TrimSpace(settings.Get("IMAGE_MODEL")); v != "" {
		return v
	}
	return ImageModelID
}

// GenerateImage turns a text prompt into an image using Atlas Cloud's async
// image API and returns a URL to the result. It calls the endpoint directly with
// the documented minimal body ({model, prompt, aspect_ratio}); the go-micro
// provider hardcodes gpt-image-2 params (quality/size/output_format/moderation)
// that nano-banana rejects as "Request parameters are invalid". Requires an
// Atlas Cloud API key.
func GenerateImage(prompt string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}
	// Keep prompts within a sane bound so the API doesn't reject overlong input.
	if len(prompt) > 2000 {
		prompt = prompt[:2000]
	}
	key := getAtlasAPIKey()
	if key == "" {
		return "", fmt.Errorf("image generation needs an Atlas Cloud API key (set ATLAS_API_KEY)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	id, err := submitImage(ctx, key, prompt)
	if err != nil {
		return "", err
	}
	return pollImage(ctx, key, id, prompt)
}

// submitImage POSTs the generation request and returns the prediction id.
func submitImage(ctx context.Context, key, prompt string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model":        imageModel(),
		"prompt":       prompt,
		"aspect_ratio": "1:1",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, atlasImageBase+"/api/v1/model/generateImage", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	out, err := imageRequest(req, key, prompt)
	if err != nil {
		return "", err
	}
	if out.Code != 200 || out.Data.ID == "" {
		return "", fmt.Errorf("image generation failed: missing prediction id")
	}
	return out.Data.ID, nil
}

// pollImage waits for the prediction to complete and returns the first image URL.
func pollImage(ctx context.Context, key, id, prompt string) (string, error) {
	url := atlasImageBase + "/api/v1/model/prediction/" + id
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("image generation timed out")
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return "", err
			}
			req.Header.Set("Authorization", "Bearer "+key)
			out, err := imageRequest(req, key, prompt)
			if err != nil {
				return "", err
			}
			switch out.Data.Status {
			case "completed":
				if len(out.Data.Outputs) > 0 && out.Data.Outputs[0] != "" {
					return out.Data.Outputs[0], nil
				}
				return "", fmt.Errorf("image generation returned no output")
			case "failed":
				msg := out.Data.Error
				if msg == "" {
					msg = "generation failed"
				}
				return "", fmt.Errorf("image generation failed: %s", msg)
			}
		}
	}
}

type imageResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		ID      string   `json:"id"`
		Status  string   `json:"status"`
		Outputs []string `json:"outputs"`
		Error   string   `json:"error"`
	} `json:"data"`
}

// imageRequest records submission and polling in the shared external-call log.
// Keep diagnostic fields, never the prompt, bearer key or signed output URLs.
func imageRequest(req *http.Request, key, prompt string) (out imageResponse, callErr error) {
	start := time.Now()
	entry := app.APILogEntry{
		Service: "images", Kind: "image", Model: imageModel(),
		Method: req.Method, URL: atlasImageBase + "/api/v1/model/generateImage",
		Outcome: "done",
	}
	if req.Method == http.MethodGet {
		entry.URL = atlasImageBase + "/api/v1/model/prediction/[id]"
	}
	clean := func(s string) string {
		if key != "" {
			s = strings.ReplaceAll(s, key, "[redacted]")
		}
		if prompt != "" {
			s = strings.ReplaceAll(s, prompt, "[prompt omitted]")
		}
		s = ProviderErrorDetail(s)
		if len(s) > 2048 {
			s = s[:2048] + "..."
		}
		return s
	}
	defer func() {
		entry.Duration = time.Since(start)
		if callErr != nil {
			entry.Outcome = "failed"
			entry.Error = clean(callErr.Error())
			callErr = fmt.Errorf("%s", entry.Error)
			app.Log("images", "%s %s: %s", entry.Method, entry.URL, entry.Error)
		}
		app.RecordExternalCall(entry)
	}()
	resp, err := imageHTTPClient.Do(req)
	if err != nil {
		entry.ErrorKind = "transport"
		return out, err
	}
	defer resp.Body.Close()
	entry.Status = resp.StatusCode
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		entry.ErrorKind = "read"
		return out, fmt.Errorf("reading image API response: %w", err)
	}
	if len(raw) > 1<<20 {
		entry.ErrorKind = "response"
		return out, fmt.Errorf("image API response exceeds 1 MB")
	}
	decodeErr := json.Unmarshal(raw, &out)
	// An allowlisted summary also makes successful/pending responses inspectable.
	summary, _ := json.Marshal(map[string]any{"code": out.Code, "status": out.Data.Status})
	entry.ResponseBody = string(summary)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		entry.ErrorKind = "http"
		detail := clean(out.Msg)
		if detail == "" {
			detail = clean(out.Data.Error)
		}
		return out, fmt.Errorf("image API error (HTTP %d): %s", resp.StatusCode, detail)
	}
	if decodeErr != nil {
		entry.ErrorKind = "decode"
		return out, fmt.Errorf("unexpected image API response: invalid JSON")
	}
	if (out.Code != 0 && out.Code != 200) || out.Data.Status == "failed" ||
		(req.Method == http.MethodPost && (out.Code != 200 || out.Data.ID == "")) {
		entry.ErrorKind = "provider"
		msg := out.Data.Error
		if msg == "" {
			msg = out.Msg
		}
		if msg == "" {
			msg = "provider returned no prediction"
		}
		return out, fmt.Errorf("image generation failed: %s", clean(msg))
	}
	if out.Data.Status == "completed" && (len(out.Data.Outputs) == 0 || out.Data.Outputs[0] == "") {
		entry.ErrorKind = "provider"
		return out, fmt.Errorf("image generation returned no output")
	}
	return out, nil
}
