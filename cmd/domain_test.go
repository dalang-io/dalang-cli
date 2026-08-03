package cmd

import "testing"

func TestDnsCheckRecordLabel(t *testing.T) {
	cases := []struct{ key, want string }{
		{"cname", "CNAME"},
		{"ownership_txt", "Ownership TXT"},
		{"ssl_txt", "SSL TXT"},
		{"unknown", "(?)"},
	}
	for _, c := range cases {
		if got := dnsCheckRecordLabel(c.key); got != c.want {
			t.Errorf("dnsCheckRecordLabel(%q) = %q, want %q", c.key, got, c.want)
		}
	}
}


func TestCmdDomainRouting(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "no args shows help", args: []string{}, wantErr: false},
		{name: "help flag", args: []string{"--help"}, wantErr: false},
		{name: "help short", args: []string{"-h"}, wantErr: false},
		{name: "unknown subcommand", args: []string{"foo"}, wantErr: true},
		{name: "enable missing name", args: []string{"enable"}, wantErr: true},
		{name: "list missing name", args: []string{"list"}, wantErr: true},
		{name: "ls missing name", args: []string{"ls"}, wantErr: true},
		{name: "add missing args", args: []string{"add"}, wantErr: true},
		{name: "add missing domain", args: []string{"add", "myvm"}, wantErr: true},
		{name: "verify missing domain", args: []string{"verify"}, wantErr: true},
		{name: "remove missing domain", args: []string{"remove"}, wantErr: true},
		{name: "rm missing domain", args: []string{"rm"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cmdDomain(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("cmdDomain(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
		})
	}
}
