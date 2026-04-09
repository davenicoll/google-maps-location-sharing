package main

import (
	"os"
	"testing"
)

func TestParseResponse(t *testing.T) {
	body, err := os.ReadFile("/tmp/test_response.txt")
	if err != nil {
		t.Fatalf("reading test data: %v", err)
	}

	locations, err := parseResponse(body)
	if err != nil {
		t.Fatalf("parseResponse: %v", err)
	}

	// Should have 3 shared people + 1 self = 4
	if len(locations) < 3 {
		t.Fatalf("expected at least 3 locations, got %d", len(locations))
	}

	// Verify Hannah's data
	var hannah *Location
	for i := range locations {
		if locations[i].Name == "Hannah Nicoll" {
			hannah = &locations[i]
			break
		}
	}
	if hannah == nil {
		t.Fatal("Hannah Nicoll not found")
	}

	if hannah.Address != "22470 Dewdney Trunk Rd, Maple Ridge, BC V2X 5Z6, Canada" {
		t.Errorf("address = %q", hannah.Address)
	}
	if hannah.Battery != 56 {
		t.Errorf("battery = %d, want 56", hannah.Battery)
	}
	if hannah.Accuracy != 2 {
		t.Errorf("accuracy = %d, want 2", hannah.Accuracy)
	}
	if hannah.Latitude == 0 || hannah.Longitude == 0 {
		t.Errorf("coords = %f, %f", hannah.Latitude, hannah.Longitude)
	}

	t.Logf("Parsed %d locations:", len(locations))
	for _, l := range locations {
		t.Logf("  %s: %s (battery: %d%%, accuracy: %dm)", l.Name, l.Address, l.Battery, l.Accuracy)
	}
}
