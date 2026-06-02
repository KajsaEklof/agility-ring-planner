package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/projects/ring-planner/schedule"
)

type scheduleRequest struct {
	CSV               string   `json:"csv"`
	ShowName          string   `json:"showName"`
	Date              string   `json:"date"`
	StartTime         string   `json:"startTime"`         // "09:00"
	NumRings          int      `json:"numRings"`          // 0 = auto
	MinRings          int      `json:"minRings"`          // floor for auto-detection
	MaxRings          int      `json:"maxRings"`          // cap for auto-detection
	MaxChangesPerRing int      `json:"maxChangesPerRing"` // 0 = default 3
	SecsPerDog        int      `json:"secsPerDog"`        // 0 = default 60
	WalkMins          int      `json:"walkMins"`          // 0 = default 10
	ChangeMins        int      `json:"changeMins"`        // 0 = default 15
	Judges            []string `json:"judges"`
}

type scheduleResponse struct {
	Success bool              `json:"success"`
	Error   string            `json:"error,omitempty"`
	Plan    *schedule.RingPlan `json:"plan,omitempty"`
}

func handleSchedule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req scheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, scheduleResponse{Success: false, Error: "invalid JSON: " + err.Error()})
		return
	}

	classes, err := schedule.ParseCSV(req.CSV)
	if err != nil {
		writeJSON(w, scheduleResponse{Success: false, Error: "CSV error: " + err.Error()})
		return
	}
	if len(classes) == 0 {
		writeJSON(w, scheduleResponse{Success: false, Error: "no classes found in CSV"})
		return
	}

	// Parse start time
	startTime := time.Date(2000, 1, 1, 9, 0, 0, 0, time.UTC)
	if req.StartTime != "" {
		if t, err2 := time.Parse("15:04", req.StartTime); err2 == nil {
			startTime = time.Date(2000, 1, 1, t.Hour(), t.Minute(), 0, 0, time.UTC)
		}
	}

	plan := schedule.Schedule(classes, schedule.ScheduleOptions{
		ShowName:          req.ShowName,
		Date:              req.Date,
		StartTime:         startTime,
		NumRings:          req.NumRings,
		MinRings:          req.MinRings,
		MaxRings:          req.MaxRings,
		MaxChangesPerRing: req.MaxChangesPerRing,
		Timing: schedule.TimingConfig{
			SecsPerDog: req.SecsPerDog,
			WalkMins:   req.WalkMins,
			ChangeMins: req.ChangeMins,
		},
		Judges: req.Judges,
	})

	writeJSON(w, scheduleResponse{Success: true, Plan: plan})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: %v", err)
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/schedule", handleSchedule)
	mux.Handle("/", http.FileServer(http.Dir("web")))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Ring Planner running → http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
