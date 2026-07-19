package ai

import (
	"context"
	"fmt"
	"strings"
	"time"

	gmai "go-micro.dev/v6/ai"

	"mu/internal/settings"
)

// ImageModelID is the Atlas Cloud text-to-image model Mu generates with.
// Override with the IMAGE_MODEL setting if you prefer another Atlas model.
const ImageModelID = "google/nano-banana-2-lite"

// imageModel returns the configured image model id.
func imageModel() string {
	if v := strings.TrimSpace(settings.Get("IMAGE_MODEL")); v != "" {
		return v
	}
	return ImageModelID
}

// GenerateImage turns a text prompt into an image using Atlas Cloud, returning
// a URL to the generated image (or a data: URI if the provider returns base64).
// It mirrors the text path in micro.go: pick the provider, pass the Atlas key,
// let go-micro handle the submit-and-poll. Requires an Atlas Cloud API key.
func GenerateImage(prompt string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}
	key := getAtlasAPIKey()
	if key == "" {
		return "", fmt.Errorf("image generation needs an Atlas Cloud API key (set ATLAS_API_KEY)")
	}

	ig := gmai.NewImage("atlascloud", gmai.WithAPIKey(key))

	// nano-banana generation is a submit-and-poll flow; give it room but cap it.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	resp, err := ig.GenerateImage(ctx, &gmai.ImageRequest{
		Prompt: prompt,
		Model:  imageModel(),
		Size:   "1024x1024",
		N:      1,
	})
	if err != nil {
		return "", err
	}
	if resp == nil || len(resp.Images) == 0 {
		return "", fmt.Errorf("no image was returned")
	}
	img := resp.Images[0]
	if img.URL != "" {
		return img.URL, nil
	}
	if img.Base64 != "" {
		return "data:image/png;base64," + img.Base64, nil
	}
	return "", fmt.Errorf("image result was empty")
}
