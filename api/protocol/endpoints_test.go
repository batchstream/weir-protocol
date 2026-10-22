package protocol

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestPortableEndpointValidation(t *testing.T) {
	input := []string{"EXAMPLE.com.:07447", "[::ffff:127.0.0.1]:7447", "[0:0::1]:7447"}
	original := slices.Clone(input)
	targets, err := CanonicalEndpoints(input)
	want := []string{"127.0.0.1:7447", "[::1]:7447", "example.com:7447"}
	if err != nil || !slices.Equal(targets, want) {
		t.Fatal(targets, err)
	}
	if !slices.Equal(input, original) {
		t.Fatal("canonicalization modified its input", input)
	}
	for _, bad := range []string{"0.0.0.0:1", "[::ffff:0.0.0.0]:1", "[::ffff:224.0.0.1]:1", "224.0.0.1:1", "[ff02::1]:1", "[::]:1", "[fe80::1%en0]:1", "bad:0", "dns:///bad:1", "bad:65536", "bad:1/path", "bad..name:1", "[bad]:1", "bad:-1", "_name:1", "中文:1", "peer :1"} {
		if _, err := CanonicalEndpoint(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	duplicate := []string{"example.com:1", "EXAMPLE.COM.:01"}
	original = slices.Clone(duplicate)
	if targets, err := CanonicalEndpoints(duplicate); err == nil || targets != nil {
		t.Fatal("canonical duplicate accepted")
	}
	if !slices.Equal(duplicate, original) {
		t.Fatal("duplicate rejection modified its input", duplicate)
	}
}

func TestCanonicalEndpointsAcceptsLargeSetsAndRejectsEmpty(t *testing.T) {
	endpoints := make([]string, 1000)
	for i := range endpoints {
		endpoints[i] = fmt.Sprintf("replica-%04d.example:7447", i)
	}
	canonical, err := CanonicalEndpoints(endpoints)
	if err != nil || len(canonical) != len(endpoints) {
		t.Fatal("large endpoint set rejected", len(canonical), err)
	}
	if _, err := CanonicalEndpoints(nil); err == nil {
		t.Fatal("empty endpoint set accepted")
	}
}

func TestDiscoveryStoreNames(t *testing.T) {
	for _, name := range []string{"data", "mongo-1", strings.Repeat("a", 128)} {
		if !ValidStoreName(name) {
			t.Errorf("valid Store name rejected: %q", name)
		}
	}
	for _, name := range []string{"", "Data", "data/path", "data?filter", "data:7447", "a-", "weir://data"} {
		if ValidStoreName(name) {
			t.Errorf("invalid Store name accepted: %q", name)
		}
	}
}
