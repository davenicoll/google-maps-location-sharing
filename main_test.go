package main

import (
	"os"
	"testing"
)

func TestParseResponse(t *testing.T) {
	body, err := os.ReadFile("testdata/response.txt")
	if err != nil {
		t.Fatalf("reading test data: %v", err)
	}

	locations, err := parseResponse(body)
	if err != nil {
		t.Fatalf("parseResponse: %v", err)
	}

	if len(locations) == 0 {
		t.Fatal("expected at least 1 location, got 0")
	}

	for _, loc := range locations {
		if loc.Name == "" {
			t.Error("location has empty name")
		}
		if loc.Latitude == 0 && loc.Longitude == 0 {
			t.Errorf("%s: both lat and lng are zero", loc.Name)
		}
		if loc.Latitude < -90 || loc.Latitude > 90 {
			t.Errorf("%s: latitude %f out of range", loc.Name, loc.Latitude)
		}
		if loc.Longitude < -180 || loc.Longitude > 180 {
			t.Errorf("%s: longitude %f out of range", loc.Name, loc.Longitude)
		}
		if loc.Timestamp <= 0 {
			t.Errorf("%s: invalid timestamp %d", loc.Name, loc.Timestamp)
		}
		if loc.Time == "" {
			t.Errorf("%s: empty formatted time", loc.Name)
		}
		if loc.Address == "" {
			t.Errorf("%s: empty address", loc.Name)
		}
		if loc.Accuracy < 0 {
			t.Errorf("%s: negative accuracy %d", loc.Name, loc.Accuracy)
		}
	}

	t.Logf("parsed %d locations", len(locations))
	for _, l := range locations {
		t.Logf("  %s: %s (battery: %d%%, accuracy: %dm)", l.Name, l.Address, l.Battery, l.Accuracy)
	}
}
