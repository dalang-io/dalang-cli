package cmd

import (
	"strings"
	"testing"
)

func TestFormatIDR(t *testing.T) {
	tests := []struct {
		name   string
		amount int64
		want   string
	}{
		{name: "zero", amount: 0, want: "Rp 0"},
		{name: "small", amount: 100, want: "Rp 100"},
		{name: "thousands", amount: 5000, want: "Rp 5.000"},
		{name: "ten thousands", amount: 20000, want: "Rp 20.000"},
		{name: "hundred thousands", amount: 150000, want: "Rp 150.000"},
		{name: "millions", amount: 1000000, want: "Rp 1.000.000"},
		{name: "large", amount: 12345678, want: "Rp 12.345.678"},
		{name: "one digit", amount: 1, want: "Rp 1"},
		{name: "two digits", amount: 50, want: "Rp 50"},
		{name: "exact thousands boundary", amount: 1000, want: "Rp 1.000"},
		{name: "negative small", amount: -500, want: "Rp -500"},
		{name: "negative thousands", amount: -50000, want: "Rp -50.000"},
		{name: "negative millions", amount: -1234567, want: "Rp -1.234.567"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatIDR(tt.amount)
			if got != tt.want {
				t.Fatalf("formatIDR(%d) = %q, want %q", tt.amount, got, tt.want)
			}
		})
	}
}

func TestFormatDate(t *testing.T) {
	tests := []struct {
		name    string
		isoDate string
		want    string
	}{
		{name: "full iso", isoDate: "2025-01-15T10:30:00Z", want: "2025-01-15"},
		{name: "date only", isoDate: "2025-01-15", want: "2025-01-15"},
		{name: "short string", isoDate: "2025", want: "2025"},
		{name: "empty", isoDate: "", want: ""},
		{name: "with timezone", isoDate: "2025-06-20T14:00:00+07:00", want: "2025-06-20"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDate(tt.isoDate)
			if got != tt.want {
				t.Fatalf("formatDate(%q) = %q, want %q", tt.isoDate, got, tt.want)
			}
		})
	}
}

func TestFormatExpiryWithDays(t *testing.T) {
	tests := []struct {
		name    string
		isoDate string
	}{
		{name: "empty returns N/A", isoDate: ""},
		{name: "unparseable", isoDate: "not-a-date"},
		{name: "valid ISO format", isoDate: "2020-01-01T00:00:00Z"},
		{name: "date only format", isoDate: "2020-01-01"},
		{name: "space format", isoDate: "2020-01-01 00:00:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatExpiryWithDays(tt.isoDate)
			if tt.isoDate == "" && got != "N/A" {
				t.Fatalf("formatExpiryWithDays(%q) = %q, want %q", tt.isoDate, got, "N/A")
			}
			if tt.isoDate != "" && got == "" {
				t.Fatalf("formatExpiryWithDays(%q) returned empty string", tt.isoDate)
			}
		})
	}
}

func TestCmdCreditRouting(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "help flag", args: []string{"--help"}, wantErr: false},
		{name: "help short", args: []string{"-h"}, wantErr: false},
		{name: "unknown subcommand", args: []string{"foo"}, wantErr: true},
		{name: "add missing amount", args: []string{"add"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cmdCredit(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("cmdCredit(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
		})
	}
}

func TestCreditAddValidation(t *testing.T) {
	tests := []struct {
		name    string
		amount  string
		wantErr bool
	}{
		{name: "non-numeric", amount: "abc", wantErr: true},
		{name: "below minimum", amount: "10", wantErr: true},
		{name: "exactly at minimum boundary", amount: "49", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := creditAdd(tt.amount)
			if (err != nil) != tt.wantErr {
				t.Fatalf("creditAdd(%q) error = %v, wantErr %v", tt.amount, err, tt.wantErr)
			}
		})
	}
}

// The amount already carries its sign — outgoing rows are stored negative — so
// the renderer must not prepend another one. It used to for "spend", which the
// customer read as "-Rp -8.800" in `dalang credit history`.
func TestRenderTransaction_SignIsNotDoubled(t *testing.T) {
	for _, tc := range []struct {
		txType string
		amount int64
		want   string
	}{
		{"spend", -8800, "Rp -8.800"},
		{"admin_debit", -30000, "Rp -30.000"},
		{"expired", -5000, "Rp -5.000"},
		{"topup", 100000, "Rp 100.000"},
		{"admin_credit", 50000, "Rp 50.000"},
	} {
		_, got := renderTransaction(tc.txType, tc.amount)
		if got != tc.want {
			t.Errorf("renderTransaction(%q, %d) = %q, want %q", tc.txType, tc.amount, got, tc.want)
		}
		if strings.Contains(got, "--") || strings.Contains(got, "-Rp -") {
			t.Errorf("%q rendered a doubled sign: %q", tc.txType, got)
		}
	}
}

// Every type the API can emit must be coloured, or a customer sees a bare row
// among coloured ones and wonders what it is. The list mirrors
// specs/10-wallet-affiliate.md.
func TestRenderTransaction_AllAPITypesAreColoured(t *testing.T) {
	incoming := []string{"topup", "commission", "refund", "signup_bonus", "admin_credit"}
	outgoing := []string{"spend", "admin_debit", "expired"}

	for _, ty := range append(append([]string{}, incoming...), outgoing...) {
		color, _ := renderTransaction(ty, 1000)
		if color == colorReset {
			t.Errorf("type %q is one the API emits but renders uncoloured", ty)
		}
	}
	for _, ty := range outgoing {
		if color, _ := renderTransaction(ty, -1000); color != colorRed {
			t.Errorf("outgoing type %q should be red, got %q", ty, color)
		}
	}
	// Anything unknown still prints rather than being dropped.
	if _, amt := renderTransaction("something_new", -1234); amt != "Rp -1.234" {
		t.Errorf("an unknown type must still show its amount, got %q", amt)
	}
}
