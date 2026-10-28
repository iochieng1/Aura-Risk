package services

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"aurarisk-backend/internal/models"
)

// Languages for risk summaries and tips (#60). English is the default and the
// language the rest of the service (alerts, logs, confidence reasons) uses.
const (
	LangEnglish = "en"
	LangSwahili = "sw"
)

// The Swahili text mirrors the tip.*, level.* and risk.summary entries in
// packages/shared/src/i18n.ts: change them together. Translations need
// review by a native speaker before launch (docs/I18N.md).
var (
	swahiliLevels = map[string]string{
		"Normal":    "Kawaida",
		"Advisory":  "Tahadhari",
		"Alert":     "Onyo",
		"Emergency": "Dharura",
	}
	// Keyed by the English tip from GetTips.
	swahiliTips = map[string]string{
		"No immediate flood threat detected.":                "Hakuna tishio la mafuriko kwa sasa.",
		"Keep drainage areas clear of debris.":               "Weka mitaro na njia za maji bila takataka.",
		"Monitor local weather updates regularly.":           "Fuatilia taarifa za hali ya hewa za eneo lako mara kwa mara.",
		"Clear gutters and storm drains near your property.": "Safisha mifereji na mitaro ya maji ya mvua karibu na makazi yako.",
		"Review your emergency contact list.":                "Hakikisha orodha yako ya namba za dharura iko tayari.",
		"Prepare an emergency go-bag with essentials.":       "Andaa mkoba wa dharura wenye mahitaji muhimu.",
		"Move vehicles to higher ground if possible.":        "Hamisha magari kwenda sehemu za juu ikiwezekana.",
		"Avoid walking or driving through floodwater.":       "Usitembee wala kuendesha gari kupitia maji ya mafuriko.",
		"Charge all devices and keep flashlights ready.":     "Chaji simu na vifaa vyako, na weka tochi tayari.",
		"Evacuate immediately if instructed by authorities.": "Hama mara moja ukiagizwa na mamlaka.",
		"Move to the highest floor of your building.":        "Panda hadi ghorofa ya juu kabisa ya jengo lako.",
		"Do NOT attempt to cross flowing water.":             "USIJARIBU kuvuka maji yanayotiririka.",
		"Call emergency services if trapped.":                "Piga simu kwa huduma za dharura ukinaswa.",
	}
)

const swahiliSummary = "Hatari ya mafuriko kwa %s kwa sasa ni %s, ikiwa na alama %d/100."

// LocalizeRisk rewrites an assessment's summary and tips in lang. Unknown
// languages and English leave it unchanged.
func LocalizeRisk(risk *models.RiskAssessment, lang string) {
	if lang != LangSwahili {
		return
	}
	risk.Summary = fmt.Sprintf(swahiliSummary, risk.Location.Name, LevelName(lang, risk.Level), risk.Score)
	tips := make([]string, len(risk.Tips))
	for i, tip := range risk.Tips {
		tips[i] = Tip(lang, tip)
	}
	risk.Tips = tips
}

// LevelName returns a risk level's name in lang, falling back to English.
func LevelName(lang, level string) string {
	if sw := swahiliLevels[level]; lang == LangSwahili && sw != "" {
		return sw
	}
	return level
}

// Tip returns one of GetTips' English tips in lang, falling back to English.
func Tip(lang, tip string) string {
	if sw := swahiliTips[tip]; lang == LangSwahili && sw != "" {
		return sw
	}
	return tip
}

// NegotiateLanguage picks the response language: an explicit lang (en or sw,
// or a tag such as sw-KE) wins, then Accept-Language by q-value, then English.
func NegotiateLanguage(lang, acceptLanguage string) string {
	if l, ok := supportedLanguage(lang); ok {
		return l
	}
	type candidate struct {
		lang string
		q    float64
	}
	var candidates []candidate
	for _, part := range strings.Split(acceptLanguage, ",") {
		tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		q := 1.0
		for _, p := range strings.Split(params, ";") {
			if name, value, ok := strings.Cut(strings.TrimSpace(p), "="); ok && strings.TrimSpace(name) == "q" {
				parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
				if err != nil {
					parsed = 0
				}
				q = parsed
			}
		}
		if l, ok := supportedLanguage(tag); ok && q > 0 {
			candidates = append(candidates, candidate{l, q})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].q > candidates[j].q })
	if len(candidates) > 0 {
		return candidates[0].lang
	}
	return LangEnglish
}

// supportedLanguage matches a BCP 47 tag on its primary subtag.
func supportedLanguage(tag string) (string, bool) {
	primary, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(tag)), "-")
	primary, _, _ = strings.Cut(primary, "_")
	switch primary {
	case LangEnglish, LangSwahili:
		return primary, true
	}
	return "", false
}
