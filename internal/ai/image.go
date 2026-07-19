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
	return pollImage(ctx, key, id)
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

	resp, err := imageHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image API error (%s): %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("unexpected image API response: %s", strings.TrimSpace(string(raw)))
	}
	if out.Code != 200 || out.Data.ID == "" {
		if out.Msg != "" {
			return "", fmt.Errorf("image generation failed: %s", out.Msg)
		}
		return "", fmt.Errorf("image generation failed")
	}
	return out.Data.ID, nil
}

// pollImage waits for the prediction to complete and returns the first image URL.
func pollImage(ctx context.Context, key, id string) (string, error) {
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
			resp, err := imageHTTPClient.Do(req)
			if err != nil {
				return "", err
			}
			raw, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var out struct {
				Data struct {
					Status  string   `json:"status"`
					Outputs []string `json:"outputs"`
					Error   string   `json:"error"`
				} `json:"data"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				continue // transient; keep polling until the deadline
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
