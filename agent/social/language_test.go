package social

import "testing"

// The tag is what the author's client said; the script is what they wrote.
//
// This is the post that reached micro.mu's feed: Japanese prose, tagged en, so
// both filters that read the tag let it through — worthParsing scanning the
// bytes for "en inside langs, and consider asking the parsed field the same
// question. Nobody was lying; a client defaulted to en. Which is the point: a
// self-declared field cannot be the only check.
func TestAJapanesePostTaggedEnglishIsNotEnglish(t *testing.T) {
	const seen = "トヨタのAI化で道路データの価値が跳ね上がるわ。Appen等の海外サイトに登録し、" +
		"スマホで標識等の動画を撮り学習データとして納品なさい。運転を「AIへの給餌」に変えるの。"
	if latinScript(seen) {
		t.Error("a post written in Japanese passed as English")
	}
}

func TestOtherScriptsAreRefusedToo(t *testing.T) {
	for name, prose := range map[string]string{
		"Chinese": "丰田的人工智能化让道路数据的价值大幅上升，请在国外网站注册并上传视频作为训练数据。",
		"Korean":  "도요타의 인공지능화로 도로 데이터의 가치가 크게 올라갑니다. 해외 사이트에 등록하세요.",
		"Arabic":  "ارتفعت قيمة بيانات الطرق بشكل كبير مع تحول تويوتا إلى الذكاء الاصطناعي.",
		"Russian": "Стоимость дорожных данных резко выросла после перехода Toyota на искусственный интеллект.",
		"Greek":   "Η αξία των δεδομένων του δρόμου αυξήθηκε απότομα με τη στροφή της Toyota.",
		"Thai":    "มูลค่าของข้อมูลถนนพุ่งสูงขึ้นอย่างมากเมื่อโตโยต้าหันมาใช้ปัญญาประดิษฐ์",
		"Hindi":   "टोयोटा के एआई की ओर बढ़ने से सड़क डेटा का मूल्य तेजी से बढ़ गया है।",
		"Hebrew":  "ערכם של נתוני הדרך עלה בחדות עם המעבר של טויוטה לבינה מלאכותית.",
	} {
		if latinScript(prose) {
			t.Errorf("%s passed as English", name)
		}
	}
}

// English survives, including the parts of English that are not plain ASCII.
func TestEnglishPasses(t *testing.T) {
	for name, prose := range map[string]string{
		"plain":      "13 million adults in England are locked out of an NHS dentist, and preventable tooth decay is the top reason children end up in hospital.",
		"accents":    "The café on the corner serves a naïve interpretation of a jalapeño, and the soufflé is a résumé of everything wrong with the place.",
		"emoji":      "Elon Musk enters the midterm chat 🙄🙄 don't think for a second this isn't deliberate 🔥",
		"punctuated": "Rates held — again. \"No change until the data moves,\" the governor said; nobody believed him.",
		"numbers":    "GDP 0.3%, CPI 2.1%, unemployment 4.4% — the whole quarter in nine digits and not one of them a surprise.",
		// An English post is still English when it names something in another
		// script. This is the case latinShare exists for.
		"quoting a name": "Toyota's road data play is the interesting one here. The company files it under 自動運転 but the strategy is plainly about mapping, not driving.",
	} {
		if !latinScript(prose) {
			t.Errorf("%s did not pass as English", name)
		}
	}
}

// A post with no letters at all is not this check's to refuse. Too short and
// too empty are decided before it, by minText.
func TestNothingToJudgeIsNotRefusedHere(t *testing.T) {
	for _, prose := range []string{"", "🔥🔥🔥", "2026 — 100% (!!)", "   "} {
		if !latinScript(prose) {
			t.Errorf("%q was refused by the script check rather than by minText", prose)
		}
	}
}

// The gap, stated so it is a decision rather than a surprise: script is not
// language, so Latin-script languages tagged as English still pass here. They
// are refused later or not at all, and closing it means identifying a language,
// which is a model call per post on a firehose sample.
func TestLatinScriptIsNotALanguageCheck(t *testing.T) {
	const portuguese = "O valor dos dados rodoviários subiu muito depois que a Toyota passou a usar inteligência artificial."
	if !latinScript(portuguese) {
		t.Error("latinScript has started identifying languages; the comment on it " +
			"and the test below it both say it does not")
	}
}
