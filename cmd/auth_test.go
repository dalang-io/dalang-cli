package cmd

import "testing"

func TestCmdAuthRouting(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cmdAuth(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("cmdAuth(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
		})
	}
}

func TestAuthLogout(t *testing.T) {
	// authLogout should succeed even when no credentials exist
	err := authLogout()
	if err != nil {
		t.Fatalf("authLogout returned error: %v", err)
	}
}
