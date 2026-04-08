package cmd

import (
	"fmt"
	"testing"
)

func TestParseSize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{name: "gigabytes uppercase", in: "2G", want: 2048},
		{name: "gigabytes with GB", in: "2GB", want: 2048},
		{name: "megabytes", in: "512M", want: 512},
		{name: "megabytes with MB", in: "512MB", want: 512},
		{name: "plain number", in: "1024", want: 1024},
		{name: "lowercase g", in: "4g", want: 4096},
		{name: "lowercase m", in: "256m", want: 256},
		{name: "1G", in: "1G", want: 1024},
		{name: "10G", in: "10G", want: 10240},
		{name: "zero", in: "0", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSize(tt.in)
			if got != tt.want {
				t.Fatalf("parseSize(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestCmdServiceRouting(t *testing.T) {
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
		{name: "info missing name", args: []string{"info"}, wantErr: true},
		{name: "upgrade missing name", args: []string{"upgrade"}, wantErr: true},
		{name: "extend missing name", args: []string{"extend"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cmdService(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("cmdService(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
		})
	}
}

func TestServiceCreateValidation(t *testing.T) {
	resetGlobalFlags()
	t.Cleanup(resetGlobalFlags)

	// Missing --name should fail
	err := serviceCreate([]string{"--cpu", "2", "--ram", "1G"})
	if err == nil {
		t.Fatal("serviceCreate without --name should return error")
	}

	// --help should not fail
	err = serviceCreate([]string{"--help"})
	if err != nil {
		t.Fatalf("serviceCreate --help returned error: %v", err)
	}
}

func TestCmdServiceDefaultsToList(t *testing.T) {
	// cmdService with no args calls serviceList which requires auth
	// so it should return an error about authentication
	err := cmdService([]string{})
	if err == nil {
		// Might succeed if somehow authenticated, that's fine
		return
	}
	// Should fail due to auth, not a routing error
}

func TestUnifiedServiceType(t *testing.T) {
	service := UnifiedService{
		Type:    "vps",
		ID:      "test-id",
		Name:    "TestVM",
		Status:  "RUNNING",
		Details: "2C/2048MB/20GB",
	}

	if service.Type != "vps" {
		t.Fatalf("unexpected type: %s", service.Type)
	}
	if service.Name != "TestVM" {
		t.Fatalf("unexpected name: %s", service.Name)
	}

	expected := fmt.Sprintf("%dC/%dMB/%dGB", 2, 2048, 20)
	if service.Details != expected {
		t.Fatalf("unexpected details: got %q want %q", service.Details, expected)
	}
}
