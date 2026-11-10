package geoip

import "testing"

func TestIsLocal(t *testing.T) {
	cases := []struct {
		ip    string
		local bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"localhost", true},
		{"LOCALHOST", true},
		{"10.0.0.5", true},
		{"172.16.3.9", true},
		{"192.168.1.20", true},
		{"169.254.10.10", true},
		{"fe80::1", true},
		{"fd00::42", true},
		{"0.0.0.0", true},
		{"::", true},
		{"::ffff:127.0.0.1", true},
		{"::ffff:10.1.2.3", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"203.0.113.7", false},
		{"", false},
		{"not-an-ip", false},
		{"172.32.0.1", false},
	}
	for _, tc := range cases {
		if got := IsLocal(tc.ip); got != tc.local {
			t.Errorf("IsLocal(%q) = %v, want %v", tc.ip, got, tc.local)
		}
	}
}

func TestClassify(t *testing.T) {
	country, resolved := Classify("192.168.0.1")
	if !resolved || country != LocalNetwork {
		t.Fatalf("Classify(local) = %q, %v", country, resolved)
	}
	country, resolved = Classify("::1")
	if !resolved || country != LocalNetwork {
		t.Fatalf("Classify(loopback6) = %q, %v", country, resolved)
	}
	if _, resolved = Classify(""); !resolved {
		t.Fatal("Classify(empty) must be resolved")
	}
	if _, resolved = Classify("not-an-ip"); !resolved {
		t.Fatal("Classify(invalid) must be resolved")
	}
	if _, resolved = Classify("8.8.8.8"); resolved {
		t.Fatal("Classify(public) must be unresolved")
	}
}
