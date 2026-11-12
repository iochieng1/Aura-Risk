package workers

import (
	"strings"
	"testing"
	"time"

	"aurarisk-backend/internal/models"
	"aurarisk-backend/internal/services"
)

// Push alerts name the subscribed place (#85).
func TestAlertMessageNamesPlace(t *testing.T) {
	sub := notifierSubscription{ID: "s1", Name: "Kibera", Lat: -1.31, Lon: 36.79}
	// Assessments are shared by nearby subscriptions, so this one may carry
	// another subscription's name; the alert must use its own.
	assessment := &models.RiskAssessment{Location: models.Location{Name: "Other Place"}, Level: "Alert", Score: 62,
		Summary: "Flood risk for Other Place is currently Alert with a risk score of 62/100.",
		Tips:    []string{"Avoid walking or driving through floodwater."}}
	msg := alertMessage("ExponentPushToken[x]", sub, assessment)
	text := msg.Title + " " + msg.Body
	if msg.Title != "Alert: Kibera" || strings.Contains(text, "Other Place") || strings.Contains(text, "Selected Location") {
		t.Fatalf("title %q, body %q", msg.Title, msg.Body)
	}
}

func TestDecide(t *testing.T) {
	// 12:00 UTC is 15:00 in Nairobi.
	noon := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	quietNow := &services.QuietHours{Start: "14:00", End: "16:00", Timezone: "Africa/Nairobi"}
	quietLater := &services.QuietHours{Start: "22:00", End: "07:00", Timezone: "Africa/Nairobi"}

	base := notifierSubscription{MinLevel: "Alert"}
	with := func(mod func(*notifierSubscription)) notifierSubscription {
		s := base
		mod(&s)
		return s
	}

	cases := []struct {
		name  string
		sub   notifierSubscription
		level string
		want  decision
	}{
		{"below threshold, nothing sent", base, "Advisory", decisionNone},
		{"below threshold after alert resets", with(func(s *notifierSubscription) {
			s.LastNotifiedLevel, s.LastNotifiedAt = "Alert", noon.Add(-time.Hour)
		}), "Normal", decisionReset},
		{"first time at threshold", base, "Alert", decisionNotify},
		{"escalation notifies", with(func(s *notifierSubscription) {
			s.LastNotifiedLevel, s.LastNotifiedAt = "Alert", noon.Add(-time.Hour)
		}), "Emergency", decisionNotify},
		{"same level recently is suppressed", with(func(s *notifierSubscription) {
			s.LastNotifiedLevel, s.LastNotifiedAt = "Alert", noon.Add(-time.Hour)
		}), "Alert", decisionNone},
		{"same level after 12h repeats", with(func(s *notifierSubscription) {
			s.LastNotifiedLevel, s.LastNotifiedAt = "Alert", noon.Add(-13*time.Hour)
		}), "Alert", decisionNotify},
		{"de-escalation lowers silently", with(func(s *notifierSubscription) {
			s.LastNotifiedLevel, s.LastNotifiedAt = "Emergency", noon.Add(-time.Hour)
		}), "Alert", decisionLower},
		{"quiet hours hold an alert", with(func(s *notifierSubscription) { s.QuietHours = quietNow }), "Alert", decisionNone},
		{"quiet hours not active", with(func(s *notifierSubscription) { s.QuietHours = quietLater }), "Alert", decisionNotify},
		{"emergency held without override", with(func(s *notifierSubscription) { s.QuietHours = quietNow }), "Emergency", decisionNone},
		{"emergency breaks through with override", with(func(s *notifierSubscription) {
			s.QuietHours, s.AllowEmergency = quietNow, true
		}), "Emergency", decisionNotify},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := decide(tc.sub, tc.level, noon); got != tc.want {
				t.Fatalf("decide = %v, want %v", got, tc.want)
			}
		})
	}
}
