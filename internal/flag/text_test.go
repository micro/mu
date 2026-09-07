package flag

import "testing"

func TestProfanityIncludesInflectionsWithoutSubstringFalsePositives(t *testing.T) {
	for _, text := range []string{"Bring it on you little fuckers", "FUCKING", "bullshit", "assholes"} {
		if !Profane(text) {
			t.Errorf("missed %q", text)
		}
	}
	for _, text := range []string{"Scunthorpe", "Shitake mushrooms", "A difficult political debate", "Classical music"} {
		if Profane(text) {
			t.Errorf("false positive: %q", text)
		}
	}
}
