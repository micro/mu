package flag

import "testing"

func TestVideoMetadataScreen(t *testing.T) {
	for _, title := range []string{
		"From GAY To STRAIGHT MUSLIM!☪️😱 *GONE WRONG*😳 #islam #muslim",
		"Kissing prank", "Casino bonus", "What the f&#117;ck",
	} {
		if !UnsuitableVideo(title, "") {
			t.Errorf("accepted unsuitable title: %s", title)
		}
	}
	for _, title := range []string{
		"Scholarly Subtitles: kindness and modesty", "Islamic views on gambling",
		"Alcohol addiction recovery", "Sexual health education",
		"Muslim and gay experiences", "How to stop watching pornography",
	} {
		if UnsuitableVideo(title, "Educational discussion") {
			t.Errorf("rejected educational or identity topic: %s", title)
		}
	}
}
