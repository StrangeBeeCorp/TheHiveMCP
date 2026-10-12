package load

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"
)

const (
	// stubKey is the API key the clients send to the stub.
	stubKey = "load-key"
	// stubLatency is how long the stub takes to answer a query, as a TheHive close by does: a run
	// measures the server's own time and its queueing on top of it.
	stubLatency = 5 * time.Millisecond
	// stubAlerts is the size of the stub's page of alerts.
	stubAlerts = 25
	// firstAlert and alertsCreated are the stub's first alert id and its alerts' date: 2026-01-01.
	firstAlert    = 4096
	alertsCreated = 1767225600000
)

// NewStub is a TheHive that knows one key: it answers the credential check, and every query
// with a page of alerts that carry the free text and dates the server wraps and converts.
func NewStub() (*httptest.Server, Target) {
	page := alerts(stubAlerts)
	user := []byte(`{"_id":"~1","_createdBy":"admin","_createdAt":1,"login":"load@example.com","name":"Load","hasKey":true,` +
		`"hasPassword":false,"hasMFA":false,"locked":false,"profile":"analyst","organisation":"load","type":"Normal","extraData":{}}`)

	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+stubKey {
			w.WriteHeader(http.StatusUnauthorized)

			return
		}

		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/v1/user/current":
			_, _ = w.Write(user)
		case "/api/v1/query":
			time.Sleep(stubLatency)

			_, _ = w.Write(page)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	return stub, Target{URL: stub.URL, Key: stubKey}
}

// alerts is n alerts as TheHive returns them.
func alerts(n int) []byte {
	page := make([]map[string]any, n)
	for i := range page {
		page[i] = map[string]any{
			"_id": fmt.Sprintf("~%d", firstAlert+i), "_type": "Alert", "_createdAt": alertsCreated + int64(i), "_createdBy": "analyst@example.com",
			"title": fmt.Sprintf("Phishing campaign %d", i), "type": "email", "description": "A user reported a suspicious message.",
			"severity": 2, "status": "New", "tags": []any{"phishing"}, "source": "mail-gateway", "sourceRef": fmt.Sprintf("MG-%d", i),
			"date": alertsCreated + int64(i), "tlp": 2, "pap": 2,
		}
	}

	data, err := json.Marshal(page)
	if err != nil {
		panic(err)
	}

	return data
}
