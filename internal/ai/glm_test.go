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
