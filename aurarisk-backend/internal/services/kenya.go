package services

import "strings"

// KenyaCounties are Kenya's 47 counties in official code order (1 Mombasa to
// 47 Nairobi). Keep in sync with KENYA_COUNTIES in packages/shared.
var KenyaCounties = []string{
	"Mombasa", "Kwale", "Kilifi", "Tana River", "Lamu", "Taita-Taveta", "Garissa", "Wajir", "Mandera",
	"Marsabit", "Isiolo", "Meru", "Tharaka-Nithi", "Embu", "Kitui", "Machakos", "Makueni", "Nyandarua",
	"Nyeri", "Kirinyaga", "Murang'a", "Kiambu", "Turkana", "West Pokot", "Samburu", "Trans Nzoia",
	"Uasin Gishu", "Elgeyo-Marakwet", "Nandi", "Baringo", "Laikipia", "Nakuru", "Narok", "Kajiado",
	"Kericho", "Bomet", "Kakamega", "Vihiga", "Bungoma", "Busia", "Siaya", "Kisumu", "Homa Bay",
	"Migori", "Kisii", "Nyamira", "Nairobi",
}

// MaxWardBytes bounds the free-text ward name.
const MaxWardBytes = 100

var countiesByKey = func() map[string]string {
	m := make(map[string]string, len(KenyaCounties))
	for _, c := range KenyaCounties {
		m[countyKey(c)] = c
	}
	return m
}()

// countyKey ignores case, a trailing "County", and how words are joined, so
// "kisumu county", "Taita Taveta" and "Muranga" all match.
func countyKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, " county")
	if s == "nairobi city" {
		s = "nairobi"
	}
	return strings.NewReplacer(" ", "", "-", "", "'", "", "’", "").Replace(s)
}

// NormalizeCounty returns the official name of a Kenyan county, or false.
func NormalizeCounty(s string) (string, bool) {
	c, ok := countiesByKey[countyKey(s)]
	return c, ok
}
