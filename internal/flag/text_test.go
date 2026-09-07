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

func TestSocialApprovalSurvivesReloadAndLateAutomaticVerdict(t *testing.T) {
	resetFlags(t)
	if err := AdminFlag("social", "quoted", "system:harmful"); err != nil {
		t.Fatal(err)
	}
	if err := Approve("social", "quoted"); err != nil {
		t.Fatal(err)
	}
	mutex.Lock()
	flags = make(map[string]*FlaggedItem)
	mutex.Unlock()
	Load()
	if !IsApproved("social", "quoted") {
		t.Fatal("approval did not survive reload")
	}
	if err := AdminFlag("social", "quoted", "system:harmful"); err != nil {
		t.Fatal(err)
	}
	if IsHidden("social", "quoted") {
		t.Fatal("late automatic verdict overruled approval")
	}
	if err := AdminFlag("social", "quoted", "operator"); err != nil {
		t.Fatal(err)
	}
	if !IsHidden("social", "quoted") {
		t.Fatal("operator could not reverse approval")
	}
}
