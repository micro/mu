package markets

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHistoryFiltersNullsAndPages(t *testing.T) {
	day := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/BTC-USD" {
			t.Errorf("wrong symbol %s", r.URL.Path)
		}
		fmt.Fprintf(w, `{"chart":{"result":[{"meta":{"currency":"USD"},"timestamp":[%d,%d,%d],"indicators":{"quote":[{"close":[10,null,12]}]}}]}}`, day.Unix(), day.AddDate(0, 0, 1).Unix(), day.AddDate(0, 0, 2).Unix())
	}))
	defer server.Close()
	old := historyURL
	historyURL = server.URL + "/"
	defer func() { historyURL = old }()
	var rsp HistoryResponse
	err := (Server{}).History(context.Background(), &HistoryRequest{Symbol: "BTC", Start: "2025-01-02", End: "2025-01-04", Limit: 1}, &rsp)
	if err != nil {
		t.Fatal(err)
	}
	if rsp.Total != 2 || len(rsp.Items) != 1 || rsp.NextOffset == nil || *rsp.NextOffset != 1 {
		t.Fatalf("bad history page: %+v", rsp)
	}
}
