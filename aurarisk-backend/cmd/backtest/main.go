// Command backtest replays past weather and river discharge through the risk
// score and reports how it would have done on known flood and non-flood days.
// See docs/BACKTEST.md.
//
//	go run ./cmd/backtest -events ../docs/backtest/kenya-2024-long-rains.csv
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"text/tabwriter"
	"time"

	"aurarisk-backend/internal/backtest"
	"aurarisk-backend/internal/services"
)

func main() {
	eventsPath := flag.String("events", "", "events CSV (required)")
	jsonPath := flag.String("json", "", "also write outcomes and metrics as JSON to this file")
	leadDays := flag.Int("lead-days", 3, "days before the event day that are scored")
	threshold := flag.String("threshold", "Alert", "level that counts as a warning: Advisory, Alert or Emergency")
	delay := flag.Duration("delay", 2*time.Second, "pause between events, to stay within Open-Meteo's free limits")
	flag.Parse()

	if *eventsPath == "" {
		flag.Usage()
		os.Exit(2)
	}
	switch *threshold {
	case "Advisory", "Alert", "Emergency":
	default:
		log.Fatalf("-threshold must be Advisory, Alert or Emergency")
	}

	f, err := os.Open(*eventsPath)
	if err != nil {
		log.Fatal(err)
	}
	events, err := backtest.ParseEvents(f)
	f.Close()
	if err != nil {
		log.Fatalf("%s: %v", *eventsPath, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	client := &services.OpenMeteoClient{
		APIKey:     os.Getenv("OPEN_METEO_API_KEY"),
		FloodURL:   os.Getenv("OPEN_METEO_FLOOD_URL"),
		ArchiveURL: os.Getenv("OPEN_METEO_ARCHIVE_URL"),
		Timeout:    30 * time.Second,
	}
	opts := backtest.Options{LeadDays: *leadDays, Threshold: *threshold}

	fmt.Fprintf(os.Stderr, "Scoring %d events with model %s...\n", len(events), services.ModelVersion)
	outcomes := backtest.Run(ctx, paced{client, *delay}, events, opts, func(o backtest.Outcome) {
		status := fmt.Sprintf("max %3d %-9s warned=%v", o.MaxScore, o.MaxLevel, o.Warned)
		if o.Err != nil {
			status = "error: " + o.Err.Error()
		}
		fmt.Fprintf(os.Stderr, "  %s %-28s flooded=%-5v %s\n", o.Date.Format("2006-01-02"), o.Place, o.Flooded, status)
	})
	metrics := backtest.Summarise(outcomes)

	fmt.Printf("\nModel %s, warning at %s or above, scoring the event day and %d days before.\n",
		services.ModelVersion, *threshold, opts.LeadDays)
	fmt.Println("Weather is ERA5 reanalysis and discharge is GloFAS, both standing in for forecasts: a perfect-forecast upper bound.")
	fmt.Println()
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "REGION\tFLOODS\tDETECTED\tRATE\tCONTROLS\tFALSE ALARMS\tRATE\tMEDIAN LEAD\tERRORS")
	for _, m := range metrics {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t%d\t%d\t%s\t%s\t%d\n", m.Region, m.Floods, m.Detected, percent(m.DetectionRate),
			m.Controls, m.FalseAlarms, percent(m.FalseAlarmRate), hours(m.MedianLeadHours), m.Errors)
	}
	tw.Flush()

	if *jsonPath != "" {
		if err := writeJSON(*jsonPath, outcomes, metrics, opts); err != nil {
			log.Fatal(err)
		}
	}
}

// paced waits between events (each event starts with an archive fetch).
type paced struct {
	*services.OpenMeteoClient
	delay time.Duration
}

func (p paced) FetchArchive(ctx context.Context, lat, lon float64, start, end string) (*services.OpenMeteoResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(p.delay):
	}
	return p.OpenMeteoClient.FetchArchive(ctx, lat, lon, start, end)
}

func percent(r *float64) string {
	if r == nil {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", *r*100)
}

func hours(h *float64) string {
	if h == nil {
		return "-"
	}
	return fmt.Sprintf("%.0f h", *h)
}

func writeJSON(path string, outcomes []backtest.Outcome, metrics []backtest.Metrics, opts backtest.Options) error {
	type outcome struct {
		Region     string `json:"region"`
		Place      string `json:"place"`
		Date       string `json:"date"`
		Flooded    bool   `json:"flooded"`
		MaxScore   int    `json:"max_score"`
		MaxLevel   string `json:"max_level"`
		Warned     bool   `json:"warned"`
		LeadHours  *int   `json:"lead_hours,omitempty"`
		HadTerrain bool   `json:"had_terrain"`
		HadRiver   bool   `json:"had_river"`
		Error      string `json:"error,omitempty"`
		Source     string `json:"source"`
	}
	report := struct {
		ModelVersion string             `json:"model_version"`
		Threshold    string             `json:"threshold"`
		LeadDays     int                `json:"lead_days"`
		Metrics      []backtest.Metrics `json:"metrics"`
		Outcomes     []outcome          `json:"outcomes"`
	}{ModelVersion: services.ModelVersion, Threshold: opts.Threshold, LeadDays: opts.LeadDays, Metrics: metrics}
	for _, o := range outcomes {
		row := outcome{Region: o.Region, Place: o.Place, Date: o.Date.Format("2006-01-02"), Flooded: o.Flooded,
			MaxScore: o.MaxScore, MaxLevel: o.MaxLevel, Warned: o.Warned, HadTerrain: o.HadTerrain, HadRiver: o.HadRiver, Source: o.Source}
		if o.Flooded && o.Warned {
			lead := o.LeadHours
			row.LeadHours = &lead
		}
		if o.Err != nil {
			row.Error = o.Err.Error()
		}
		report.Outcomes = append(report.Outcomes, row)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
