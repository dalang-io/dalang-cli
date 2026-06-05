package cmd

import "testing"

// TestCmdExecArgValidation covers the argument handling that runs before any
// network call, so it needs no auth or API.
func TestCmdExecArgValidation(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "no args", args: nil, wantErr: true},
		{name: "only vm name", args: []string{"vm"}, wantErr: true},
		{name: "help long", args: []string{"--help"}, wantErr: false},
		{name: "help short", args: []string{"-h"}, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := cmdExec(tt.args); (err != nil) != tt.wantErr {
				t.Fatalf("cmdExec(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
		})
	}
}
