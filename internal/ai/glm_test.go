package ai

import "testing"

func TestGLMModelsUseAtlasAndKeepTheirNames(t *testing.T) {
	clearProviders(t)
	t.Setenv("ATLAS_API_KEY", "test")
	t.Setenv("ATLAS_MODEL", ModelGLMFlash)
	for _, id := range []string{ModelGLM, ModelGLMFlash} {
		p, _, _, err := resolveProvider(id)
		if err != nil || p != ProviderAtlasCloud {
			t.Fatalf("%s: provider %s: %v", id, p, err)
		}
		if !Offered(id) {
			t.Errorf("%s not offered", id)
		}
		if got := modelFor(ProviderAtlasCloud, id); got != id {
			t.Errorf("%s changed to %s", id, got)
		}
		if _, ok := priceOf(id); !ok {
			t.Errorf("%s unpriced", id)
		}
	}
	if got := LabelFor(ModelGLMFlash); got != "GLM 5.3 Flash" {
		t.Fatal(got)
	}
}

func TestGLMUsesSelectedProviderAndMenu(t *testing.T) {
	clearProviders(t)
	t.Setenv("ATLAS_API_KEY", "test-atlas")
	t.Setenv("OPENROUTER_API_KEY", "test-router")
	for _, provider := range []string{ProviderAtlasCloud, ProviderOpenRouter} {
		t.Setenv("AI_PROVIDER", provider)
		for _, choice := range Choices() {
			if choice.Provider != provider {
				t.Fatalf("%s offers %s", provider, choice.Provider)
			}
		}
		for _, id := range []string{ModelGLM, ModelGLMFlash, "z-ai/glm-5.3", "z-ai/glm-5.3-flash"} {
			p, _, _, err := resolveProvider(id)
			if err != nil || p != provider {
				t.Fatalf("%s resolved %s: %v", id, p, err)
			}
			mapped := modelFor(p, id)
			if !Offered(mapped) {
				t.Fatalf("mapped model %s missing from menu", mapped)
			}
		}
	}
}
