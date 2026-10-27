package backtest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aurarisk-backend/internal/services"
)

const csvHeader = "region,place,lat,lon,date,flooded,source\n"

func TestParseEvents(t *testing.T) {
	events, err := ParseEvents(strings.NewReader(csvHeader +
		"kisumu,Ahero,-0.1740,34.9190,2024-04-23,yes,https://reliefweb.int/x\n" +
		"kisumu,Ahero,-0.1740,34.9190,2024-01-25,no,control: dry season\n"))
	if err != nil || len(events) != 2 || !events[0].Flooded || events[1].Flooded || events[0].Date.Day() != 23 {
		t.Fatalf("events %+v err %v", events, err)
	}
}

func TestParseEventsRejectsBadRows(t *testing.T) {
	for name, row := range map[string]string{
		"flood without a source": "kisumu,Ahero,-0.17,34.91,2024-04-23,yes,\n",
		"bad date":               "kisumu,Ahero,-0.17,34.91,23/04/2024,yes,https://x\n",
		"bad flooded":            "kisumu,Ahero,-0.17,34.91,2024-04-23,maybe,https://x\n",
		"lat out of range":       "kisumu,Ahero,-91,34.91,2024-04-23,no,\n",
		"no place":               "kisumu,,-0.17,34.91,2024-04-23,no,\n",
	} {
		if _, err := ParseEvents(strings.NewReader(csvHeader + row)); err == nil || !strings.Contains(err.Error(), "line 2") {
			t.Errorf("%s: err = %v, want a line 2 error", name, err)
		}
	}
	if _, err := ParseEvents(strings.NewReader("place,date\n")); err == nil {
		t.Error("expected a header error")
	}
}

// fakeSource serves an hourly series in which rain falls only during
// rainFrom..rainTo (local, naive), and a river that stays at baseline.
type fakeSource struct {
	rainFrom, rainTo time.Time
	riverErr         error
}

func (f fakeSource) FetchArchive(ctx context.Context, lat, lon float64, start, end string) (*services.OpenMeteoResponse, error) {
	from, _ := time.Parse(dateLayout, start)
	to, _ := time.Parse(dateLayout, end)
	var w services.OpenMeteoResponse
	for at := from; at.Before(to.AddDate(0, 0, 1)); at = at.Add(time.Hour) {
		rain := 0.0
		if !at.Before(f.rainFrom) && at.Before(f.rainTo) {
			rain = 12
		}
		w.Hourly.Time = append(w.Hourly.Time, at.Format("2006-01-02T15:04"))
		w.Hourly.Precipitation = append(w.Hourly.Precipitation, rain)
		w.Hourly.Rain = append(w.Hourly.Rain, rain)
		w.Hourly.SoilMoisture = append(w.Hourly.SoilMoisture, 0.3)
		w.Hourly.WeatherCode = append(w.Hourly.WeatherCode, 0)
	}
	return &w, nil
}

func (f fakeSource) FetchRiverHistory(ctx context.Context, lat, lon float64, start, end string) ([]services.RiverSeries, error) {
	if f.riverErr != nil {
		return nil, f.riverErr
	}
	from, _ := time.Parse(dateLayout, start)
	to, _ := time.Parse(dateLayout, end)
	var s services.RiverSeries
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		v := 40.0
		s.Daily.Time = append(s.Daily.Time, d.Format(dateLayout))
		s.Daily.RiverDischarge = append(s.Daily.RiverDischarge, &v)
	}
	return []services.RiverSeries{s}, nil
}

func (f fakeSource) FetchTerrain(ctx context.Context, lat, lon float64) (*services.Terrain, error) {
	return &services.Terrain{Elevation: 1150}, nil
}

func day(s string) time.Time {
	t, _ := time.Parse(dateLayout, s)
	return t
}

func TestRunDetectsRainBeforeAFloodAndMeasuresLead(t *testing.T) {
	// Heavy rain from 30 hours before the flood day.
	src := fakeSource{rainFrom: day("2024-04-22").Add(-6 * time.Hour), rainTo: day("2024-04-23")}
	flood := Event{Region: "kisumu", Place: "Ahero", Date: day("2024-04-23"), Flooded: true}
	o := Run(context.Background(), src, []Event{flood}, Options{}, nil)[0]

	if o.Err != nil || !o.Warned || !o.HadRiver || !o.HadTerrain {
		t.Fatalf("outcome %+v", o)
	}
	// The forecast window sees rain 6 hours ahead, so the first warning comes
	// at or before the rain starts: at least 30 hours ahead.
	if o.LeadHours < 30 || o.LeadHours > 36 {
		t.Fatalf("lead = %d h, want 30-36", o.LeadHours)
	}
}

func TestRunCountsDryDaysAsNoWarning(t *testing.T) {
	src := fakeSource{}
	control := Event{Region: "kisumu", Place: "Ahero", Date: day("2024-01-25")}
	if o := Run(context.Background(), src, []Event{control}, Options{}, nil)[0]; o.Warned || o.Err != nil {
		t.Fatalf("outcome %+v", o)
	}
}

func TestRunScoresWithoutRiverData(t *testing.T) {
	src := fakeSource{riverErr: errors.New("flood API down")}
	o := Run(context.Background(), src, []Event{{Region: "r", Place: "p", Date: day("2024-04-23")}}, Options{}, nil)[0]
	if o.Err != nil || o.HadRiver {
		t.Fatalf("outcome %+v", o)
	}
}

func TestSummarise(t *testing.T) {
	outcomes := []Outcome{
		{Event: Event{Region: "kisumu", Flooded: true}, Warned: true, LeadHours: 10},
		{Event: Event{Region: "kisumu", Flooded: true}, Warned: true, LeadHours: 30},
		{Event: Event{Region: "kisumu", Flooded: true}},
		{Event: Event{Region: "kisumu"}, Warned: true},
		{Event: Event{Region: "kisumu"}},
		{Event: Event{Region: "nairobi", Flooded: true}, Err: errors.New("down")},
	}
	m := Summarise(outcomes)
	if len(m) != 3 || m[0].Region != "kisumu" || m[1].Region != "nairobi" || m[2].Region != "all" {
		t.Fatalf("regions %+v", m)
	}
	k := m[0]
	if k.Floods != 3 || k.Detected != 2 || *k.DetectionRate != 2.0/3 || *k.FalseAlarmRate != 0.5 || *k.MedianLeadHours != 20 {
		t.Fatalf("kisumu %+v", k)
	}
	if n := m[1]; n.Errors != 1 || n.DetectionRate != nil || n.MedianLeadHours != nil {
		t.Fatalf("nairobi %+v", n)
	}
}
