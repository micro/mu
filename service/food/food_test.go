package food

// The failures that would matter.
//
// Almost everything here is about saying nothing rather than saying something
// wrong. A missing allergen field is not "no allergens", a Scottish restaurant
// has no number out of five, and an FHRS hygiene score of zero is the best
// there is rather than the worst.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func stub(t *testing.T, target *string, body string, status int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	old := *target
	*target = srv.URL + "/"
	t.Cleanup(func() { *target = old })
}

const noAllergenData = `{"status":1,"product":{"product_name":"Mystery Biscuits","brands":"Someone",
 "ingredients_text":"Flour, sugar","allergens_tags":[],"traces_tags":[],
 "nutriscore_grade":"unknown","nova_group":0,"nutriments":{}}}`

func TestSilenceAboutAllergensIsNotReassurance(t *testing.T) {
	stub(t, &offProductURL, noAllergenData, http.StatusOK)

	var rsp ProductResponse
	if err := (Server{}).Product(context.Background(), &ProductRequest{Barcode: "5000168034928"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "not recorded") {
		t.Errorf("did not say the allergen data is missing:\n%s", rsp.Text)
	}
	// And it must not invent a grade out of "unknown".
	if strings.Contains(rsp.Text, "Nutri-Score") {
		t.Errorf("printed a Nutri-Score for a product that has none:\n%s", rsp.Text)
	}
}

const withAllergens = `{"status":1,"product":{"product_name":"Peanut Bar","brands":"B",
 "allergens_tags":["en:peanuts","en:milk"],"traces_tags":["en:sesame-seeds"],
 "nutriscore_grade":"d","nova_group":4,
 "nutriments":{"energy-kcal_100g":512.4,"salt_100g":"0.35"}}}`

func TestAllergensComeBeforeCalories(t *testing.T) {
	stub(t, &offProductURL, withAllergens, http.StatusOK)

	var rsp ProductResponse
	if err := (Server{}).Product(context.Background(), &ProductRequest{Barcode: "1234567"}, &rsp); err != nil {
		t.Fatal(err)
	}
	contains := strings.Index(rsp.Text, "Contains: peanuts")
	per100 := strings.Index(rsp.Text, "Per 100g")
	if contains < 0 {
		t.Fatalf("lost the allergens:\n%s", rsp.Text)
	}
	if per100 >= 0 && contains > per100 {
		t.Errorf("buried the allergens below the nutrition:\n%s", rsp.Text)
	}
	if !strings.Contains(rsp.Text, "May contain: sesame seeds") {
		t.Errorf("lost the traces:\n%s", rsp.Text)
	}
	// A nutriment contributed as a string is still a number.
	if !strings.Contains(rsp.Text, "salt 0.4g") {
		t.Errorf("dropped a nutriment that arrived as a string:\n%s", rsp.Text)
	}
	if !strings.Contains(rsp.Text, "ultra-processed") {
		t.Errorf("left NOVA as a bare number:\n%s", rsp.Text)
	}
}

func TestABarcodeIsCheckedBeforeSpendingARequest(t *testing.T) {
	// The unreachable URL is the assertion: a malformed barcode must not
	// produce a request at all.
	old := offProductURL
	offProductURL = "http://127.0.0.1:1/"
	t.Cleanup(func() { offProductURL = old })

	for _, bad := range []string{"", "abc", "12345", "123456789012345", "50001-68034"} {
		var rsp ProductResponse
		err := (Server{}).Product(context.Background(), &ProductRequest{Barcode: bad}, &rsp)
		if err == nil {
			t.Errorf("accepted %q as a barcode", bad)
			continue
		}
		if !strings.Contains(err.Error(), "not a barcode") {
			t.Errorf("for %q got %v, want a complaint about the barcode", bad, err)
		}
	}
}

func TestAnAbsentProductSaysWhyItMightBeAbsent(t *testing.T) {
	stub(t, &offProductURL, `{"status":0}`, http.StatusOK)

	var rsp ProductResponse
	err := (Server{}).Product(context.Background(), &ProductRequest{Barcode: "0000000000000"}, &rsp)
	if err == nil {
		t.Fatal("reported a product that is not there")
	}
	if !strings.Contains(err.Error(), "contributed to by") {
		t.Errorf("did not explain that the database is not exhaustive: %v", err)
	}
}

func TestABusyDatabaseIsNotAnUnknownError(t *testing.T) {
	stub(t, &offProductURL, "", http.StatusServiceUnavailable)

	var rsp ProductResponse
	err := (Server{}).Product(context.Background(), &ProductRequest{Barcode: "5000168034928"}, &rsp)
	if err == nil || !strings.Contains(err.Error(), "busy") {
		t.Errorf("got %v, want something a caller can act on", err)
	}
}

const scottish = `{"establishments":[{"BusinessName":"The Chippy","BusinessType":"Takeaway",
 "RatingValue":"Pass","RatingDate":"2026-03-04T00:00:00","AddressLine1":"1 High St",
 "PostCode":"EH1 1AA","SchemeType":"FHIS","scores":{}}]}`

func TestScotlandHasNoNumberOutOfFive(t *testing.T) {
	stub(t, &fsaURL, scottish, http.StatusOK)

	var rsp HygieneResponse
	if err := (Server{}).Hygiene(context.Background(), &HygieneRequest{Name: "The Chippy"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rsp.Text, "out of 5") {
		t.Errorf("scored a Scottish business out of five:\n%s", rsp.Text)
	}
	if !strings.Contains(rsp.Text, "pass") {
		t.Errorf("lost the actual rating:\n%s", rsp.Text)
	}
}

const spotless = `{"establishments":[{"BusinessName":"Caffe Nero","BusinessType":"Cafe",
 "RatingValue":"5","RatingDate":"2026-05-14T00:00:00","AddressLine1":"60 Trafalgar Square",
 "PostCode":"WC2N 5DS","SchemeType":"FHRS",
 "scores":{"Hygiene":0,"Structural":5,"ConfidenceInManagement":0}}]}`

func TestAZeroHygieneScoreIsTheBestOneNotTheWorst(t *testing.T) {
	stub(t, &fsaURL, spotless, http.StatusOK)

	var rsp HygieneResponse
	if err := (Server{}).Hygiene(context.Background(), &HygieneRequest{Name: "Caffe Nero"}, &rsp); err != nil {
		t.Fatal(err)
	}
	// In FHRS a component score counts what was found wrong, so zero is
	// spotless. Printing the raw number would read as a disaster.
	if strings.Contains(rsp.Text, "hygiene 0") || strings.Contains(rsp.Text, "Hygiene: 0") {
		t.Errorf("printed a raw inverted score:\n%s", rsp.Text)
	}
	if !strings.Contains(rsp.Text, "hygiene very good") {
		t.Errorf("did not translate the score:\n%s", rsp.Text)
	}
}

const notInspected = `{"establishments":[{"BusinessName":"New Place","RatingValue":"AwaitingInspection",
 "SchemeType":"FHRS","scores":{}}]}`

func TestNotYetInspectedIsNotAFailure(t *testing.T) {
	stub(t, &fsaURL, notInspected, http.StatusOK)

	var rsp HygieneResponse
	if err := (Server{}).Hygiene(context.Background(), &HygieneRequest{Name: "New Place"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "not yet inspected") {
		t.Errorf("rendered an uninspected business as rated:\n%s", rsp.Text)
	}
}

func TestHygieneNeedsSomethingToGoOn(t *testing.T) {
	var rsp HygieneResponse
	err := (Server{}).Hygiene(context.Background(), &HygieneRequest{}, &rsp)
	if err == nil {
		t.Fatal("searched every food business in the country")
	}
}

func TestSearchDropsGradesItDoesNotHave(t *testing.T) {
	stub(t, &offSearchURL, `{"hits":[
	  {"code":"1","product_name":"Oat Milk","brands":["Planet"],"nutriscore_grade":"unknown"},
	  {"code":"2","product_name":"","brands":["Nameless"],"nutriscore_grade":"a"},
	  {"code":"3","product_name":"Boring Oat Milk","brands":["Boring"],"nutriscore_grade":"c"}]}`,
		http.StatusOK)

	var rsp SearchResponse
	if err := (Server{}).Search(context.Background(), &SearchRequest{Query: "oat milk"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rsp.Text, "UNKNOWN") {
		t.Errorf("printed 'unknown' as a Nutri-Score:\n%s", rsp.Text)
	}
	if !strings.Contains(rsp.Text, "Nutri-Score C") {
		t.Errorf("lost a real grade:\n%s", rsp.Text)
	}
	// A contributed row with no name is not a search result.
	if strings.Contains(rsp.Text, "Nameless") {
		t.Errorf("offered a nameless product:\n%s", rsp.Text)
	}
	// Every hit carries the barcode, because that is what food_product takes.
	if !strings.Contains(rsp.Text, "barcode 3") {
		t.Errorf("did not print the barcode:\n%s", rsp.Text)
	}
}
