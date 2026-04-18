package discovery

import (
	"testing"
)

func TestSplitByDestination(t *testing.T) {
	records := []ShipmentRecord{
		{ConsigneeStdName: "A", ConsigneeCountry: "UNITED STATES"},
		{ConsigneeStdName: "B", ConsigneeCountry: "UNITED STATES"},
		{ConsigneeStdName: "C", ConsigneeCountry: "CANADA"},
		{ConsigneeStdName: "D", ConsigneeCountry: "US"},
	}
	groups := SplitByDestination(records)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups (US, CA), got %d", len(groups))
	}
	for _, g := range groups {
		if g.DestinationCountry != "US" && g.DestinationCountry != "CA" {
			t.Errorf("unexpected country %q", g.DestinationCountry)
		}
	}
}

func TestDeriveMarketMetadata_DominantHSCode(t *testing.T) {
	records := []ShipmentRecord{
		{ConsigneeStdName: "A", HSCode: "6802.91.00.00", ShipperCountry: "TURKEY"},
		{ConsigneeStdName: "A", HSCode: "680291", ShipperCountry: "TURKEY"},
		{ConsigneeStdName: "B", HSCode: "6802.93", ShipperCountry: "TURKEY"},
	}
	group := ShipmentGroup{DestinationCountry: "US", Records: records}
	meta := DeriveMarketMetadata(group)

	if meta.DominantHSCode != "6802.91" {
		t.Errorf("dominant HS: expected 6802.91, got %q", meta.DominantHSCode)
	}
	if meta.OriginCountry != "TR" {
		t.Errorf("origin: expected TR, got %q", meta.OriginCountry)
	}
	if meta.OriginShare < 0.99 {
		t.Errorf("origin share: expected ~1.0, got %f", meta.OriginShare)
	}
	if meta.ImporterCount != 2 {
		t.Errorf("importer count: expected 2, got %d", meta.ImporterCount)
	}
	if meta.ShipmentCount != 3 {
		t.Errorf("shipment count: expected 3, got %d", meta.ShipmentCount)
	}
}

func TestPercentileDate(t *testing.T) {
	records := []ShipmentRecord{
		{ConsigneeStdName: "A", ArrivalDate: "2019-01-01"}, // outlier
	}
	for i := 0; i < 98; i++ {
		records = append(records, ShipmentRecord{
			ConsigneeStdName: "B",
			ArrivalDate:      "2025-06-15",
		})
	}
	records = append(records, ShipmentRecord{
		ConsigneeStdName: "C",
		ArrivalDate:      "2026-04-18",
	})

	group := ShipmentGroup{DestinationCountry: "US", Records: records}
	meta := DeriveMarketMetadata(group)

	// 5th percentile should skip the 2019 outlier (1/100 = 1%)
	if meta.ShipmentFromDate.Year() < 2025 {
		t.Errorf("5th percentile should skip 2019 outlier, got %v", meta.ShipmentFromDate)
	}
}

func TestCountryToISO2(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"UNITED STATES", "US"},
		{"Turkey", "TR"},
		{"TÜRKIYE", "TR"},
		{"US", "US"},
		{"tr", "TR"},
		{"United States(EN)", "US"},
		{"", ""},
		{"UNKNOWN_PLACE", ""},
	}
	for _, tt := range tests {
		got := countryToISO2(tt.in)
		if got != tt.want {
			t.Errorf("countryToISO2(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
