package handlers

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"aurarisk-backend/internal/metrics"
	"aurarisk-backend/internal/models"
	"aurarisk-backend/internal/services"
	"github.com/gin-gonic/gin"
)

// SMS and USSD access through Africa's Talking (#61), for feature phones.
// See docs/SMS_USSD.md.

// ChannelConfig enables the SMS and USSD callbacks.
type ChannelConfig struct {
	// Must match the callback URL's "key" query parameter.
	CallbackSecret string
	// Salt for the phone number hash.
	PhoneSalt string
	// Sends SMS replies; nil logs them instead (the USSD menu works without).
	SMS services.SMSSender
	// Requests per phone per minute across both channels.
	PerMinute int
	// Reports per phone per hour.
	ReportsPerHour int
	// Scores a county headquarters; services.GenerateRiskAssessment by default.
	Assess func(ctx context.Context, lat, lon float64, name string) (*models.RiskAssessment, error)
}

var (
	channels       *ChannelConfig
	channelLimiter *RateLimiter
)

// SetChannels enables the SMS and USSD callbacks; nil disables them (404).
func SetChannels(cfg *ChannelConfig) {
	if cfg != nil && cfg.Assess == nil {
		cfg.Assess = services.GenerateRiskAssessment
	}
	if cfg != nil && cfg.ReportsPerHour <= 0 {
		cfg.ReportsPerHour = 5
	}
	channels = cfg
	channelLimiter = nil
	if cfg != nil {
		channelLimiter = NewRateLimiter("channels", cfg.PerMinute)
	}
}

// channelPhone checks the callback secret and returns the caller's phone
// hash, or answers the request itself and returns "".
func channelPhone(c *gin.Context, field string) string {
	if channels == nil {
		NotFound(c)
		return ""
	}
	if subtle.ConstantTimeCompare([]byte(c.Query("key")), []byte(channels.CallbackSecret)) != 1 {
		respondError(c, http.StatusForbidden, "invalid callback key")
		return ""
	}
	phone := c.PostForm(field)
	if services.NormalizePhone(phone) == "" {
		respondError(c, http.StatusBadRequest, field+" is required")
		return ""
	}
	return hex.EncodeToString(services.HashPhone(channels.PhoneSalt, phone))
}

// allowChannel applies the per-phone limit.
func allowChannel(phoneHash string) bool {
	if channelLimiter == nil {
		return true
	}
	if ok, _ := channelLimiter.allow(phoneHash); !ok {
		metrics.ObserveRateLimited("channels")
		return false
	}
	return true
}

// USSDCallback answers Africa's Talking USSD requests. text holds every
// answer so far, joined by "*"; replies start with CON (more input) or END.
//
//	""                    -> choose a language
//	L                     -> 1 check risk, 2 report flooding
//	L*1 / L*1*county      -> ask for the county / the county's risk
//	L*2 / ...*county      -> ask for the county / the ward ("0" skips)
//	L*2*county*ward       -> choose a category
//	L*2*county*ward*n     -> store the report
func USSDCallback(c *gin.Context) {
	phoneHash := channelPhone(c, "phoneNumber")
	if phoneHash == "" {
		return
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	if !allowChannel(phoneHash) {
		c.String(http.StatusOK, "END "+channelText(ussdLang(c.PostForm("text")), "too_many"))
		return
	}
	c.String(http.StatusOK, ussdReply(c, phoneHash, c.PostForm("text")))
}

func ussdReply(c *gin.Context, phoneHash, text string) string {
	if text == "" {
		return "CON " + channelText("en", "welcome")
	}
	parts := strings.Split(text, "*")
	lang := ussdLang(text)
	if lang == "" {
		return "END " + channelText("en", "invalid")
	}
	t := func(key string) string { return channelText(lang, key) }
	if len(parts) == 1 {
		return "CON " + t("menu")
	}

	switch parts[1] {
	case "1":
		if len(parts) == 2 {
			return "CON " + t("ask_county")
		}
		county, ok := services.NormalizeCounty(parts[2])
		if !ok || len(parts) > 3 {
			return "END " + t("unknown_county")
		}
		return "END " + countyRiskMessage(c.Request.Context(), lang, county)
	case "2":
		if len(parts) == 2 {
			return "CON " + t("ask_county")
		}
		county, ok := services.NormalizeCounty(parts[2])
		if !ok {
			return "END " + t("unknown_county")
		}
		if len(parts) == 3 {
			return "CON " + t("ask_ward")
		}
		ward := strings.TrimSpace(parts[3])
		if ward == "0" {
			ward = ""
		}
		if len(ward) > services.MaxWardBytes {
			return "END " + t("ward_too_long")
		}
		if len(parts) == 4 {
			return "CON " + t("ask_category")
		}
		category, ok := ussdCategories[parts[4]]
		if !ok || len(parts) > 5 {
			return "END " + t("invalid")
		}
		return "END " + storeChannelReport(c, phoneHash, lang, services.SourceUSSD, county, ward, category, "")
	}
	return "END " + t("invalid")
}

var ussdCategories = map[string]string{"1": "flooding", "2": "road_blocked", "3": "water_rising", "4": "drainage_issue"}

// SMSCallback handles Africa's Talking inbound SMS: "RISK <county>" or
// "FLOOD <county>, <ward>, <note>" (and Swahili keywords), and texts the
// answer back.
func SMSCallback(c *gin.Context) {
	phoneHash := channelPhone(c, "from")
	if phoneHash == "" {
		return
	}
	cmd := services.ParseSMS(c.PostForm("text"))
	var reply string
	if !allowChannel(phoneHash) {
		reply = channelText(cmd.Lang, "too_many")
	} else {
		switch cmd.Kind {
		case "risk":
			reply = countyRiskMessage(c.Request.Context(), cmd.Lang, cmd.County)
		case "report":
			reply = storeChannelReport(c, phoneHash, cmd.Lang, services.SourceSMS, cmd.County, cmd.Ward, cmd.Category, cmd.Note)
		default:
			reply = channelText(cmd.Lang, "sms_help")
		}
	}

	if channels.SMS == nil {
		slog.Info("sms reply not sent: no SMS sender configured", "request_id", GetRequestID(c))
	} else if err := channels.SMS.Send(c.Request.Context(), c.PostForm("from"), reply); err != nil {
		slog.Error("sms reply failed", "request_id", GetRequestID(c), "error", err)
	}
	c.Status(http.StatusOK)
}

// countyRiskMessage scores the county headquarters and describes it in one
// short message (USSD screens hold 182 characters).
func countyRiskMessage(ctx context.Context, lang, county string) string {
	hq := services.CountyHeadquarters[county]
	risk, err := channels.Assess(ctx, hq[0], hq[1], county)
	if err != nil {
		slog.Error("channel risk failed", "county", county, "error", err)
		return channelText(lang, "risk_unavailable")
	}
	tip := ""
	if len(risk.Tips) > 0 {
		tip = " " + channelTip(lang, risk.Tips[0])
	}
	return fmt.Sprintf(channelText(lang, "risk"), county, channelLevel(lang, risk.Level), risk.Score) +
		tip + " " + channelText(lang, "decision_support")
}

// storeChannelReport files a report for the phone's account, at the county
// headquarters, through the same duplicate detection and moderation as app
// reports. It returns the message for the reporter.
func storeChannelReport(c *gin.Context, phoneHash, lang, source, county, ward, category, note string) string {
	t := func(key string) string { return channelText(lang, key) }
	if len(ward) > services.MaxWardBytes {
		return t("ward_too_long")
	}
	if note == "" {
		note = fmt.Sprintf("%s reported by %s", categoryLabels[category], strings.ToUpper(source))
	}
	if len(note) > 2000 {
		note = note[:2000]
	}
	ctx := c.Request.Context()
	account, err := channelAccount(ctx, phoneHash)
	if err != nil {
		internalChannelError(c, err)
		return t("failed")
	}
	var recent int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM community_reports WHERE account_id = $1 AND created_at > NOW() - INTERVAL '1 hour'
	`, account).Scan(&recent); err != nil {
		internalChannelError(c, err)
		return t("failed")
	}
	if recent >= channels.ReportsPerHour {
		return t("too_many")
	}

	name := county
	var wardPtr *string
	if ward != "" {
		name = ward + ", " + county
		wardPtr = &ward
	}
	hq := services.CountyHeadquarters[county]
	if _, err := storeReport(c, newReport{
		Location:  models.Location{Name: name, Lat: hq[0], Lon: hq[1]},
		Category:  category,
		Note:      note,
		AccountID: account,
		County:    &county,
		Ward:      wardPtr,
		Source:    source,
	}); err != nil {
		internalChannelError(c, err)
		return t("failed")
	}
	return t("thanks")
}

var categoryLabels = map[string]string{
	"flooding": "Flooding", "road_blocked": "Road blocked", "water_rising": "Water rising", "drainage_issue": "Drainage issue",
}

// channelAccount returns the account for a phone hash, creating it once.
func channelAccount(ctx context.Context, phoneHash string) (string, error) {
	hash, err := hex.DecodeString(phoneHash)
	if err != nil {
		return "", err
	}
	var id string
	err = db.QueryRowContext(ctx, `SELECT account_id FROM channel_reporters WHERE phone_hash = $1`, hash).Scan(&id)
	if err == nil || !errors.Is(err, sql.ErrNoRows) {
		return id, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, `INSERT INTO accounts DEFAULT VALUES RETURNING id`).Scan(&id); err != nil {
		return "", err
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO channel_reporters (phone_hash, account_id) VALUES ($1, $2) ON CONFLICT (phone_hash) DO NOTHING
	`, hash, id)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Another request created it first; use theirs.
		tx.Rollback()
		return id, db.QueryRowContext(ctx, `SELECT account_id FROM channel_reporters WHERE phone_hash = $1`, hash).Scan(&id)
	}
	return id, tx.Commit()
}

func internalChannelError(c *gin.Context, err error) {
	slog.Error("channel request failed", "request_id", GetRequestID(c), "error", err)
}

// channelMessages are the SMS and USSD texts. Keep them short: a USSD screen
// holds 182 characters and an SMS 160 before it splits.
var channelMessages = map[string]map[string]string{
	"en": {
		"welcome":          "Welcome to AuraRisk / Karibu AuraRisk\n1. English\n2. Kiswahili",
		"menu":             "AuraRisk flood risk\n1. Check flood risk\n2. Report flooding",
		"ask_county":       "Enter your county, e.g. Kisumu:",
		"ask_ward":         "Enter your ward, or 0 to skip:",
		"ask_category":     "What is happening?\n1. Flooding\n2. Road blocked\n3. Water rising\n4. Drainage blocked",
		"unknown_county":   "County not recognised. Please try again with the county name, e.g. Kisumu.",
		"ward_too_long":    "Ward name is too long. Please try again.",
		"invalid":          "Invalid choice. Please try again.",
		"too_many":         "Too many requests. Please try again later.",
		"failed":           "Sorry, we could not save your report. Please try again later.",
		"thanks":           "Thank you. Your report was received and will be checked before it is shown publicly.",
		"risk":             "AuraRisk %s: %s flood risk (%d/100).",
		"risk_unavailable": "Flood risk is unavailable right now. Please try again later.",
		"decision_support": "Decision support only; follow official warnings.",
		"sms_help":         "AuraRisk: send RISK <county> for flood risk, or FLOOD <county>, <ward>, <what you see> to report. Kiswahili: HATARI / MAFURIKO.",
	},
	"sw": {
		"welcome":          "Welcome to AuraRisk / Karibu AuraRisk\n1. English\n2. Kiswahili",
		"menu":             "AuraRisk hatari ya mafuriko\n1. Angalia hatari ya mafuriko\n2. Ripoti mafuriko",
		"ask_county":       "Andika kaunti yako, mfano Kisumu:",
		"ask_ward":         "Andika wadi yako, au 0 kuruka:",
		"ask_category":     "Nini kinatokea?\n1. Mafuriko\n2. Barabara imefungwa\n3. Maji yanapanda\n4. Mtaro umeziba",
		"unknown_county":   "Kaunti haijatambuliwa. Tafadhali jaribu tena kwa jina la kaunti, mfano Kisumu.",
		"ward_too_long":    "Jina la wadi ni refu mno. Tafadhali jaribu tena.",
		"invalid":          "Chaguo si sahihi. Tafadhali jaribu tena.",
		"too_many":         "Maombi mengi mno. Tafadhali jaribu tena baadaye.",
		"failed":           "Samahani, hatukuweza kuhifadhi ripoti yako. Tafadhali jaribu tena baadaye.",
		"thanks":           "Asante. Ripoti yako imepokelewa na itakaguliwa kabla ya kuonyeshwa hadharani.",
		"risk":             "AuraRisk %s: hatari ya mafuriko ni %s (%d/100).",
		"risk_unavailable": "Hatari ya mafuriko haipatikani kwa sasa. Tafadhali jaribu tena baadaye.",
		"decision_support": "Ni msaada wa maamuzi tu; fuata tahadhari rasmi.",
		"sms_help":         "AuraRisk: tuma HATARI <kaunti> kujua hatari ya mafuriko, au MAFURIKO <kaunti>, <wadi>, <unachoona> kuripoti. English: RISK / FLOOD.",
	},
}

func channelText(lang, key string) string {
	if msgs, ok := channelMessages[lang]; ok {
		return msgs[key]
	}
	return channelMessages["en"][key]
}

// ussdLang reads the language chosen on the first USSD screen.
func ussdLang(text string) string {
	first, _, _ := strings.Cut(text, "*")
	switch first {
	case "1":
		return "en"
	case "2":
		return "sw"
	}
	return ""
}

var swahiliLevels = map[string]string{"Normal": "Kawaida", "Advisory": "Tahadhari", "Alert": "Onyo", "Emergency": "Dharura"}

func channelLevel(lang, level string) string {
	if lang == "sw" {
		return swahiliLevels[level]
	}
	return level
}

func channelTip(lang, tip string) string {
	return tip
}
