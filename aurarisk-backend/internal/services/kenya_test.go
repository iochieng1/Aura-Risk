package services

import "testing"

func TestKenyaHas47DistinctCounties(t *testing.T) {
	if len(KenyaCounties) != 47 || len(countiesByKey) != 47 {
		t.Fatalf("%d counties, %d distinct keys, want 47", len(KenyaCounties), len(countiesByKey))
	}
}

func TestNormalizeCounty(t *testing.T) {
	for in, want := range map[string]string{
		"Kisumu": "Kisumu", "kisumu county": "Kisumu", " TAITA TAVETA ": "Taita-Taveta",
		"Muranga": "Murang'a", "Murang’a County": "Murang'a", "homa-bay": "Homa Bay", "Nairobi City County": "Nairobi",
	} {
		if got, ok := NormalizeCounty(in); !ok || got != want {
			t.Errorf("NormalizeCounty(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "Kampala", "Kisumu City", "Nyanza"} {
		if _, ok := NormalizeCounty(bad); ok {
			t.Errorf("NormalizeCounty(%q) matched", bad)
		}
	}
}
