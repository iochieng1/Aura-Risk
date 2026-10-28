package services

import (
	"bytes"
	"testing"
)

func TestCountyHeadquartersCoverEveryCounty(t *testing.T) {
	if len(CountyHeadquarters) != len(KenyaCounties) {
		t.Fatalf("%d headquarters for %d counties", len(CountyHeadquarters), len(KenyaCounties))
	}
	for _, county := range KenyaCounties {
		hq, ok := CountyHeadquarters[county]
		// Kenya spans about 4.7°S to 5°N and 33.9°E to 41.9°E.
		if !ok || hq[0] < -4.8 || hq[0] > 5 || hq[1] < 33.9 || hq[1] > 42 {
			t.Errorf("%s: %v", county, hq)
		}
	}
}

func TestNormalizeAndHashPhone(t *testing.T) {
	for _, in := range []string{"+254 712 345678", "0712345678", "254712345678", "(+254) 712-345-678"} {
		if got := NormalizePhone(in); got != "254712345678" {
			t.Errorf("NormalizePhone(%q) = %q", in, got)
		}
	}
	a, b := HashPhone("salt", "+254712345678"), HashPhone("salt", "0712345678")
	if !bytes.Equal(a, b) || bytes.Equal(a, HashPhone("other", "0712345678")) || len(a) != 32 {
		t.Fatal("hash must depend on the normalized number and the salt")
	}
}

func TestParseSMS(t *testing.T) {
	cases := map[string]SMSCommand{
		"risk kisumu":     {Kind: "risk", Lang: "en", County: "Kisumu"},
		"HATARI Homa Bay": {Kind: "risk", Lang: "sw", County: "Homa Bay"},
		"RISK Atlantis":   {Kind: "help", Lang: "en"},
		"hello":           {Kind: "help", Lang: "en"},
		"MSAADA":          {Kind: "help", Lang: "sw"},
		"FLOOD Kisumu, Nyalenda A, water over the road": {
			Kind: "report", Lang: "en", County: "Kisumu", Ward: "Nyalenda A", Category: "flooding", Note: "water over the road",
		},
		"MAFURIKO Tana River nyumba zimejaa maji": {
			Kind: "report", Lang: "sw", County: "Tana River", Category: "flooding", Note: "nyumba zimejaa maji",
		},
		"ROAD Uasin Gishu County, Kapseret": {Kind: "report", Lang: "en", County: "Uasin Gishu", Ward: "Kapseret", Category: "road_blocked"},
		"DRAIN nowhere, x, y":               {Kind: "help", Lang: "en"},
	}
	for in, want := range cases {
		if got := ParseSMS(in); got != want {
			t.Errorf("ParseSMS(%q) = %+v, want %+v", in, got, want)
		}
	}
}
