package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/dalang-io/dalang-cli/internal/api"
)

// Paying for something with credits, the way a client that is not the dashboard must.
//
// The server debits ITS OWN price and treats a figure below it as fraud (the account is
// suspended). So this never works a price out: it asks POST /credits/quote, shows the
// answer, and pays exactly that. See api.dalang.io specs/10 ("Quote before paying").

type creditQuote struct {
	Net        int64 `json:"net"`
	PPNRate    int   `json:"ppn_rate"`
	PPN        int64 `json:"ppn"`
	Gross      int64 `json:"gross"`
	Balance    int64 `json:"balance"`
	Sufficient bool  `json:"sufficient"`
	Shortfall  int64 `json:"shortfall"`
}

type creditPayment struct {
	ProductType string
	Description string
	Metadata    map[string]interface{}
	// MaxGross refuses the payment when the quoted total is above it (0 = no limit): a
	// price the agent did not expect is a reason to stop, not to pay.
	MaxGross int64
}

type creditReceipt struct {
	ExternalID string `json:"external_id"`
	Net        int64  `json:"net"`
	Gross      int64  `json:"gross"`
	NewBalance int64  `json:"new_balance"`
}

func quoteCredits(client *api.Client, productType string, metadata map[string]interface{}) (*creditQuote, error) {
	resp, err := client.Post("/credits/quote", map[string]interface{}{"product_type": productType, "metadata": metadata})
	if err != nil {
		return nil, fmt.Errorf("could not get a price: %w", err)
	}
	var out struct {
		Data creditQuote `json:"data"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		return nil, fmt.Errorf("could not read the price: %w", err)
	}
	return &out.Data, nil
}

// payWithQuote quotes, checks the quote against what the caller will accept and what the
// wallet holds, asks for confirmation, then pays exactly the quoted net.
func payWithQuote(client *api.Client, p creditPayment) (*creditReceipt, error) {
	q, err := quoteCredits(client, p.ProductType, p.Metadata)
	if err != nil {
		return nil, err
	}
	say("Price: %s + PPN %s = %s   (credits: %s)", formatIDR(q.Net), formatIDR(q.PPN), formatIDR(q.Gross), formatIDR(q.Balance))

	if p.MaxGross > 0 && q.Gross > p.MaxGross {
		return nil, fmt.Errorf("the price is %s, above --max-price %s: not paying", formatIDR(q.Gross), formatIDR(p.MaxGross))
	}
	if !q.Sufficient {
		return nil, fmt.Errorf("not enough credits: need %s, have %s (short by %s). Top up with 'dalang credit topup'", formatIDR(q.Gross), formatIDR(q.Balance), formatIDR(q.Shortfall))
	}
	if err := requireConfirmation(fmt.Sprintf("Pay %s from credits for: %s?", formatIDR(q.Gross), p.Description)); err != nil {
		return nil, err
	}

	resp, err := client.Post("/credits/pay", map[string]interface{}{
		"product_type": p.ProductType,
		"amount":       q.Net,
		"description":  p.Description,
		"metadata":     p.Metadata,
	})
	if err != nil {
		return nil, fmt.Errorf("payment failed: %w", err)
	}
	var out struct {
		Data struct {
			ExternalID string `json:"external_id"`
			Amount     int64  `json:"amount"`
			NewBalance int64  `json:"new_balance"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		return nil, fmt.Errorf("payment went through but the reply could not be read: %w", err)
	}
	return &creditReceipt{ExternalID: out.Data.ExternalID, Net: out.Data.Amount, Gross: q.Gross, NewBalance: out.Data.NewBalance}, nil
}
