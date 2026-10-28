package services

import (
	"strings"
	"testing"

	"aurarisk-backend/internal/models"
)

func TestEveryTipHasSwahili(t *testing.T) {
	for _, level := range []string{"Normal", "Advisory", "Alert", "Emergency"} {
		if swahiliLevels[level] == "" {
			t.Errorf("level %s has no Swahili name", level)
		}
		tips := GetTips(level)
		if len(tips) == 0 {
			t.Fatalf("no tips for %s", level)
		}
		for _, tip := range tips {
			if strings.TrimSpace(swahiliTips[tip]) == "" {
				t.Errorf("%s tip %q has no Swahili translation", level, tip)
			}
		}
	}
}

func TestLocalizeRisk(t *testing.T) {
	risk := &models.RiskAssessment{
		Location: models.Location{Name: "Kisumu"},
		Score:    80,
		Level:    "Emergency",
		Summary:  "english",
		Tips:     GetTips("Emergency"),
	}
	english := append([]string(nil), risk.Tips...)

	LocalizeRisk(risk, LangEnglish)
	if risk.Summary != "english" {
		t.Fatal("English changed the summary")
	}

	LocalizeRisk(risk, LangSwahili)
	if risk.Summary != "Hatari ya mafuriko kwa Kisumu kwa sasa ni Dharura, ikiwa na alama 80/100." {
		t.Fatalf("summary %q", risk.Summary)
	}
	if risk.Tips[2] != "USIJARIBU kuvuka maji yanayotiririka." {
		t.Fatalf("tips %v", risk.Tips)
	}
	// The original slice (shared with whoever built it) is untouched.
	if GetTips("Emergency")[2] != english[2] {
		t.Fatal("GetTips was modified")
	}
}

func TestNegotiateLanguage(t *testing.T) {
	cases := []struct {
		lang, accept, want string
	}{
		{"", "", "en"},
		{"sw", "en", "sw"},
		{"sw-KE", "", "sw"},
		{"EN", "sw", "en"},
		{"fr", "sw-KE,sw;q=0.9", "sw"},
		{"", "fr-FR, sw;q=0.8, en;q=0.5", "sw"},
		{"", "en;q=0.3, sw;q=0.7", "sw"},
		{"", "sw;q=0, en", "en"},
		{"", "sw;q=x", "en"},
		{"", "de, fr", "en"},
		{"", "en-GB,sw;q=0.9", "en"},
	}
	for _, c := range cases {
		if got := NegotiateLanguage(c.lang, c.accept); got != c.want {
			t.Errorf("NegotiateLanguage(%q, %q) = %q, want %q", c.lang, c.accept, got, c.want)
		}
	}
}
