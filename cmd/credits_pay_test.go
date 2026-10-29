package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dalang-io/dalang-cli/internal/api"
	"github.com/dalang-io/dalang-cli/internal/config"
)

// creditsServer answers /credits/quote with `quote` and records what /credits/pay got.
func creditsServer(t *testing.T, quote string) (paid *map[string]interface{}) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	got := map[string]interface{}{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/credits/quote":
			w.Write([]byte(quote))
		case "/credits/pay":
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &got)
			w.Write([]byte(`{"success":true,"data":{"external_id":"CRD-1","amount":25000,"new_balance":72250}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DALANG_API_URL", srv.URL)
	if err := config.SaveCredentials(&config.Credentials{AccessToken: "tok", ExpiresAt: 4102444800}); err != nil {
		t.Fatal(err)
	}
	return &got
}

const okQuote = `{"success":true,"data":{"net":25000,"ppn_rate":11,"ppn":2750,"gross":27750,"balance":100000,"sufficient":true,"shortfall":0}}`

func payer(t *testing.T) *api.Client {
	t.Helper()
	c, err := api.NewAuthenticatedClient()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

var subscription = creditPayment{
	ProductType: "deployment_subscription",
	Description: "App Hosting bulanan - acme/shop",
	Metadata:    map[string]interface{}{"type": "deployment_subscription", "deployment_id": "dep-1", "billing_period": 1},
}

// The amount sent to /credits/pay is the QUOTED net, never anything the CLI worked out.
func TestPayWithQuote_PaysExactlyTheQuotedNet(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	yesFlag = true
	paid := creditsServer(t, okQuote)

	r, err := payWithQuote(payer(t), subscription)
	if err != nil {
		t.Fatal(err)
	}
	if (*paid)["amount"] != float64(25000) || (*paid)["product_type"] != "deployment_subscription" {
		t.Errorf("sent %v, want amount 25000 (the quoted net)", *paid)
	}
	if r.ExternalID != "CRD-1" || r.Gross != 27750 || r.NewBalance != 72250 {
		t.Errorf("receipt %+v", r)
	}
}

func TestPayWithQuote_StopsBeforePayingWhenThePriceIsAboveTheLimit(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	yesFlag = true
	paid := creditsServer(t, okQuote)

	p := subscription
	p.MaxGross = 20000
	if _, err := payWithQuote(payer(t), p); err == nil || !strings.Contains(err.Error(), "above --max-price") {
		t.Errorf("a price over the limit: %v", err)
	}
	if len(*paid) != 0 {
		t.Errorf("paid anyway: %v", *paid)
	}
}

func TestPayWithQuote_ShortOnCreditsSaysBySkippingThePayment(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	yesFlag = true
	paid := creditsServer(t, `{"success":true,"data":{"net":25000,"ppn_rate":11,"ppn":2750,"gross":27750,"balance":10000,"sufficient":false,"shortfall":17750}}`)

	_, err := payWithQuote(payer(t), subscription)
	if err == nil || !strings.Contains(err.Error(), "not enough credits") || !strings.Contains(err.Error(), "17.750") {
		t.Errorf("short of credits: %v", err)
	}
	if len(*paid) != 0 {
		t.Errorf("tried to pay with too few credits: %v", *paid)
	}
}

func TestPayWithQuote_NeedsConfirmationWhenNobodyCanAnswer(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)
	yesFlag = false
	paid := creditsServer(t, okQuote)

	if _, err := payWithQuote(payer(t), subscription); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("no --yes, no terminal: %v", err)
	}
	if len(*paid) != 0 {
		t.Errorf("paid without confirmation: %v", *paid)
	}
}
