package handlers

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"aurarisk-backend/internal/models"
	"aurarisk-backend/internal/services"
	"github.com/gin-gonic/gin"
)

type fakeSMS struct {
	mu   sync.Mutex
	sent []string
}

func (f *fakeSMS) Send(_ context.Context, to, message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, to+": "+message)
	return nil
}

func (f *fakeSMS) last() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sent) == 0 {
		return ""
	}
	return f.sent[len(f.sent)-1]
}

func channelRouter(t *testing.T) (*gin.Engine, *fakeSMS) {
	t.Helper()
	r := curationRouter(t)
	sms := &fakeSMS{}
	SetChannels(&ChannelConfig{
		CallbackSecret: "callback-secret",
		PhoneSalt:      fmt.Sprintf("salt-%d", rand.Int()),
		SMS:            sms,
		PerMinute:      1000,
		ReportsPerHour: 2,
		Assess: func(_ context.Context, lat, lon float64, name string) (*models.RiskAssessment, error) {
			return &models.RiskAssessment{Score: 62, Level: "Alert", Tips: []string{"Avoid walking or driving through floodwater."}}, nil
		},
	})
	t.Cleanup(func() { SetChannels(nil) })
	r.POST("/api/channels/ussd", USSDCallback)
	r.POST("/api/channels/sms", SMSCallback)
	return r, sms
}

func postForm(r *gin.Engine, path string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestUSSDFlow(t *testing.T) {
	r, _ := channelRouter(t)
	phone := fmt.Sprintf("+2547%08d", rand.Intn(1e8))
	ussd := func(text string) string {
		t.Helper()
		w := postForm(r, "/api/channels/ussd?key=callback-secret", url.Values{
			"sessionId": {"s1"}, "serviceCode": {"*384*123#"}, "phoneNumber": {phone}, "text": {text},
		})
		if w.Code != http.StatusOK {
			t.Fatalf("%q: %d %s", text, w.Code, w.Body)
		}
		return w.Body.String()
	}

	for text, want := range map[string]string{
		"":              "CON Welcome",
		"1":             "CON AuraRisk flood risk",
		"2":             "CON AuraRisk hatari",
		"1*1":           "CON Enter your county",
		"1*1*kisumu":    "END AuraRisk Kisumu: Alert flood risk (62/100). Avoid walking",
		"2*1*Kisumu":    "END AuraRisk Kisumu: hatari ya mafuriko ni Onyo (62/100). Usitembee wala",
		"1*1*Nyanza":    "END County not recognised",
		"9":             "END Invalid choice",
		"1*2*Siaya":     "CON Enter your ward",
		"1*2*Siaya*0":   "CON What is happening?",
		"1*2*Siaya*0*9": "END Invalid choice",
	} {
		if got := ussd(text); !strings.HasPrefix(got, want) {
			t.Errorf("%q: %q, want prefix %q", text, got, want)
		}
	}

	ward := fmt.Sprintf("Ward %d", rand.Int())
	if got := ussd("1*2*Siaya*" + ward + "*2"); !strings.HasPrefix(got, "END Thank you") {
		t.Fatalf("report: %q", got)
	}
	var source, category, note string
	var approximate bool
	var lat, lon float64
	var account string
	if err := db.QueryRow(`
		SELECT source, category, note, location_approximate, lat, lon, account_id FROM community_reports WHERE ward = $1
	`, ward).Scan(&source, &category, &note, &approximate, &lat, &lon, &account); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM accounts WHERE id = $1`, account) })
	hq := services.CountyHeadquarters["Siaya"]
	if source != "ussd" || category != "road_blocked" || !approximate || lat != hq[0] || lon != hq[1] || note != "Road blocked reported by USSD" {
		t.Fatalf("stored %s %s %q approx=%v at %v,%v", source, category, note, approximate, lat, lon)
	}

	// The number is only stored hashed, and maps to one account.
	var linked int
	db.QueryRow(`SELECT COUNT(*) FROM channel_reporters WHERE account_id = $1`, account).Scan(&linked)
	if linked != 1 {
		t.Fatalf("%d channel_reporters rows", linked)
	}
	var leaked bool
	db.QueryRow(`SELECT EXISTS (SELECT 1 FROM community_reports WHERE note LIKE '%' || $1 || '%' OR location_name LIKE '%' || $1 || '%')`,
		phone[4:]).Scan(&leaked)
	if leaked {
		t.Fatal("phone number stored in a report")
	}

	// Second report this hour is allowed, a third isn't (ReportsPerHour 2).
	ussd("1*2*Siaya*" + ward + "-b*1")
	if got := ussd("1*2*Siaya*" + ward + "-c*1"); !strings.HasPrefix(got, "END Too many") {
		t.Fatalf("third report: %q", got)
	}
}

func TestChannelCallbacksRequireKey(t *testing.T) {
	r, _ := channelRouter(t)
	form := url.Values{"phoneNumber": {"+254700000000"}, "text": {""}}
	if w := postForm(r, "/api/channels/ussd?key=wrong", form); w.Code != http.StatusForbidden {
		t.Fatalf("wrong key: %d", w.Code)
	}
	if w := postForm(r, "/api/channels/ussd?key=callback-secret", url.Values{"text": {""}}); w.Code != http.StatusBadRequest {
		t.Fatalf("no phone: %d", w.Code)
	}
	SetChannels(nil)
	if w := postForm(r, "/api/channels/ussd?key=callback-secret", form); w.Code != http.StatusNotFound {
		t.Fatalf("disabled: %d", w.Code)
	}
}

func TestSMSCommands(t *testing.T) {
	r, sms := channelRouter(t)
	phone := fmt.Sprintf("+2547%08d", rand.Intn(1e8))
	send := func(text string) string {
		t.Helper()
		w := postForm(r, "/api/channels/sms?key=callback-secret", url.Values{"from": {phone}, "to": {"12345"}, "text": {text}})
		if w.Code != http.StatusOK {
			t.Fatalf("%q: %d %s", text, w.Code, w.Body)
		}
		return strings.TrimPrefix(sms.last(), phone+": ")
	}

	if got := send("RISK Kisumu"); !strings.HasPrefix(got, "AuraRisk Kisumu: Alert flood risk (62/100).") {
		t.Errorf("risk: %q", got)
	}
	if got := send("HATARI Kisumu"); !strings.Contains(got, "Onyo") {
		t.Errorf("hatari: %q", got)
	}
	if got := send("what?"); !strings.Contains(got, "send RISK <county>") {
		t.Errorf("help: %q", got)
	}

	note := fmt.Sprintf("water over the bridge %d", rand.Int())
	if got := send("FLOOD Homa Bay, Kendu Bay, " + note); !strings.HasPrefix(got, "Thank you") {
		t.Fatalf("report: %q", got)
	}
	var county, ward, source, account string
	if err := db.QueryRow(`SELECT county, ward, source, account_id FROM community_reports WHERE note = $1`, note).
		Scan(&county, &ward, &source, &account); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM accounts WHERE id = $1`, account) })
	if county != "Homa Bay" || ward != "Kendu Bay" || source != "sms" {
		t.Fatalf("stored %s / %s / %s", county, ward, source)
	}

	// Public listings flag the location as approximate.
	hq := services.CountyHeadquarters["Homa Bay"]
	w := call(t, r, http.MethodGet, fmt.Sprintf("/api/reports?lat=%f&lon=%f&radius=1&county=Homa%%20Bay", hq[0], hq[1]), "", nil)
	if !strings.Contains(w.Body.String(), `"location_approximate":true`) {
		t.Fatalf("listing: %s", w.Body)
	}
}
