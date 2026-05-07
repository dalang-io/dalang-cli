package cmd

import "testing"

func TestParseScpEndpoint(t *testing.T) {
	cases := []struct {
		in       string
		isRemote bool
		vps      string
		path     string
	}{
		{"./local.txt", false, "", "./local.txt"},
		{"local.txt", false, "", "local.txt"},
		{"/etc/passwd", false, "", "/etc/passwd"},
		{"MyVM:/opt/app", true, "MyVM", "/opt/app"},
		{"my-vm-1:/var/log/app.log", true, "my-vm-1", "/var/log/app.log"},
		// Leading slash → local even if path contains a colon.
		{"/etc:weird/file", false, "", "/etc:weird/file"},
		// Colon at very start (empty host) → local.
		{":foo", false, "", ":foo"},
		// Empty input → local empty.
		{"", false, "", ""},
		// Relative path with colon after slash should still be remote per scp rule
		// because it does NOT start with "/" and the part before ":" is non-empty.
		{"host:./relative", true, "host", "./relative"},
	}
	for _, tc := range cases {
		got := parseScpEndpoint(tc.in)
		if got.IsRemote != tc.isRemote || got.VPSName != tc.vps || got.Path != tc.path {
			t.Errorf("parseScpEndpoint(%q) = {remote=%v vps=%q path=%q}, want {remote=%v vps=%q path=%q}",
				tc.in, got.IsRemote, got.VPSName, got.Path, tc.isRemote, tc.vps, tc.path)
		}
	}
}

func TestScpEndpoint_String(t *testing.T) {
	cases := []struct {
		ep   scpEndpoint
		want string
	}{
		{scpEndpoint{Path: "./x"}, "./x"},
		{scpEndpoint{IsRemote: true, VPSName: "MyVM", Path: "/etc"}, "MyVM:/etc"},
	}
	for _, tc := range cases {
		if got := tc.ep.String(); got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
	}
}
