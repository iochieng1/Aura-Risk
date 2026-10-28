package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"strings"
	"unicode"
)

// CountyHeadquarters are approximate coordinates of each county's
// headquarters town, used to place SMS and USSD reports and to check a
// county's flood risk from a feature phone (#61). Risk is for that town, not
// the whole county.
var CountyHeadquarters = map[string][2]float64{
	"Mombasa": {-4.04, 39.67}, "Kwale": {-4.17, 39.45}, "Kilifi": {-3.63, 39.85},
	"Tana River": {-1.50, 40.03}, "Lamu": {-2.27, 40.90}, "Taita-Taveta": {-3.40, 38.36},
	"Garissa": {-0.45, 39.65}, "Wajir": {1.75, 40.06}, "Mandera": {3.94, 41.86},
	"Marsabit": {2.33, 37.99}, "Isiolo": {0.35, 37.58}, "Meru": {0.05, 37.65},
	"Tharaka-Nithi": {-0.30, 37.65}, "Embu": {-0.53, 37.45}, "Kitui": {-1.37, 38.01},
	"Machakos": {-1.52, 37.26}, "Makueni": {-1.78, 37.63}, "Nyandarua": {-0.27, 36.38},
	"Nyeri": {-0.42, 36.95}, "Kirinyaga": {-0.50, 37.28}, "Murang'a": {-0.72, 37.15},
	"Kiambu": {-1.17, 36.83}, "Turkana": {3.12, 35.60}, "West Pokot": {1.24, 35.11},
	"Samburu": {1.10, 36.70}, "Trans Nzoia": {1.02, 35.00}, "Uasin Gishu": {0.52, 35.27},
	"Elgeyo-Marakwet": {0.67, 35.51}, "Nandi": {0.20, 35.10}, "Baringo": {0.49, 35.74},
	"Laikipia": {0.27, 36.54}, "Nakuru": {-0.30, 36.07}, "Narok": {-1.08, 35.87},
	"Kajiado": {-1.85, 36.78}, "Kericho": {-0.37, 35.28}, "Bomet": {-0.78, 35.34},
	"Kakamega": {0.28, 34.75}, "Vihiga": {0.08, 34.72}, "Bungoma": {0.56, 34.56},
	"Busia": {0.46, 34.11}, "Siaya": {0.06, 34.29}, "Kisumu": {-0.09, 34.77},
	"Homa Bay": {-0.53, 34.46}, "Migori": {-1.06, 34.47}, "Kisii": {-0.68, 34.77},
	"Nyamira": {-0.56, 34.94}, "Nairobi": {-1.29, 36.82},
}

// Report sources.
const (
	SourceApp  = "app"
	SourceSMS  = "sms"
	SourceUSSD = "ussd"
)

// HashPhone returns HMAC-SHA256(salt, number) of a normalized phone number,
// so the same number always maps to the same reporter without storing it.
func HashPhone(salt, phone string) []byte {
	mac := hmac.New(sha256.New, []byte(salt))
	mac.Write([]byte(NormalizePhone(phone)))
	return mac.Sum(nil)
}

// NormalizePhone reduces a Kenyan number to international form without the
// plus: "+254 712 345678", "0712345678" and "254712345678" all become
// "254712345678". Other numbers keep their digits only.
func NormalizePhone(phone string) string {
	digits := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, phone)
	if len(digits) == 10 && digits[0] == '0' {
		return "254" + digits[1:]
	}
	return digits
}

// SMSCommand is a parsed inbound SMS.
type SMSCommand struct {
	// "risk", "report" or "help".
	Kind     string
	Lang     string
	County   string
	Ward     string
	Category string
	Note     string
}

// smsKeywords maps the first word of an SMS to its command, category and
// language.
var smsKeywords = map[string]struct{ kind, category, lang string }{
	"RISK":     {"risk", "", "en"},
	"HATARI":   {"risk", "", "sw"},
	"FLOOD":    {"report", "flooding", "en"},
	"MAFURIKO": {"report", "flooding", "sw"},
	"ROAD":     {"report", "road_blocked", "en"},
	"BARABARA": {"report", "road_blocked", "sw"},
	"RISING":   {"report", "water_rising", "en"},
	"MAJI":     {"report", "water_rising", "sw"},
	"DRAIN":    {"report", "drainage_issue", "en"},
	"MTARO":    {"report", "drainage_issue", "sw"},
	"HELP":     {"help", "", "en"},
	"MSAADA":   {"help", "", "sw"},
}

// ParseSMS reads "RISK Kisumu" or "FLOOD Kisumu, Nyalenda A, water over the
// road" (Swahili keywords too). With commas the parts are county, ward and
// note; without, the county is the longest leading run of words that names
// one and the rest is the note. An unknown county yields Kind "help".
func ParseSMS(text string) SMSCommand {
	text = strings.TrimSpace(text)
	first, rest, _ := strings.Cut(text, " ")
	kw, ok := smsKeywords[strings.ToUpper(first)]
	if !ok {
		return SMSCommand{Kind: "help", Lang: "en"}
	}
	cmd := SMSCommand{Kind: kw.kind, Lang: kw.lang, Category: kw.category}
	if cmd.Kind == "help" {
		return cmd
	}
	rest = strings.TrimSpace(rest)

	var countyText string
	if strings.Contains(rest, ",") {
		parts := strings.SplitN(rest, ",", 3)
		countyText = parts[0]
		if len(parts) > 1 {
			cmd.Ward = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			cmd.Note = strings.TrimSpace(parts[2])
		}
	} else {
		words := strings.Fields(rest)
		for n := min(len(words), 3); n > 0; n-- {
			if _, ok := NormalizeCounty(strings.Join(words[:n], " ")); ok {
				countyText, cmd.Note = strings.Join(words[:n], " "), strings.Join(words[n:], " ")
				break
			}
		}
	}
	county, ok := NormalizeCounty(countyText)
	if !ok {
		return SMSCommand{Kind: "help", Lang: cmd.Lang}
	}
	cmd.County = county
	return cmd
}
