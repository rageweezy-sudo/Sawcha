package main

import (
	"math"
	"net/http"
	"sort"
	"strings"
)

// ExperimentReport aggregates repeated *real captures*. It never claims that
// statistical significance or a game FPS gain can be inferred from one run.
type ExperimentGroup struct {
	Profile     string  `json:"profile"`
	Runs        int     `json:"runs"`
	Frames      int     `json:"frames"`
	MedianFPS   float64 `json:"medianFPS"`
	MedianLow1  float64 `json:"medianLow1"`
	MedianP99ms float64 `json:"medianP99ms"`
	P99MADms    float64 `json:"p99MADms"`
}

type ExperimentReport struct {
	Game             string            `json:"game"`
	Scene            string            `json:"scene"`
	Baseline         *ExperimentGroup  `json:"baseline,omitempty"`
	Candidate        *ExperimentGroup  `json:"candidate,omitempty"`
	Groups           []ExperimentGroup `json:"groups"`
	ExcludedCaptures int               `json:"excludedCaptures"`
	Status           string            `json:"status"`
	Details          string            `json:"details"`
	AvgFPSDeltaPct   float64           `json:"avgFPSDeltaPct"`
	Low1DeltaPct     float64           `json:"low1DeltaPct"`
	P99DeltaPct      float64           `json:"p99DeltaPct"`
	Caveats          []string          `json:"caveats"`
}

func medianFloat(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

func buildExperimentReport(history []Benchmark, game, scene, before, after string) ExperimentReport {
	rep := ExperimentReport{
		Game: strings.TrimSpace(game), Scene: strings.TrimSpace(scene),
		Status: "NEEDS BASELINE", Details: "Record at least three captures for each of two profiles in the same named scene.",
		Caveats: []string{
			"Use the same game build, map/scene, resolution, settings, warm-up, and capture duration.",
			"Monitor temperatures, clock throttling, background activity and capture-tool overhead.",
			"Three runs give useful screening evidence, not a controlled scientific proof or guarantee of better FPS.",
		},
	}
	if rep.Game == "" && len(history) > 0 {
		rep.Game = history[0].Game
	}
	if rep.Game == "" {
		rep.Game = "Not selected"
	}
	// When no scene is selected, pick the latest named scene *for that game*.
	if rep.Scene == "" {
		for _, b := range history {
			if strings.EqualFold(strings.TrimSpace(b.Game), rep.Game) && strings.TrimSpace(b.Label) != "" {
				rep.Scene = b.Label
				break
			}
		}
	}
	groups := map[string][]Benchmark{}
	display := map[string]string{}
	order := []string{}
	for _, b := range history {
		if !strings.EqualFold(strings.TrimSpace(b.Game), rep.Game) || rep.Scene == "" || !strings.EqualFold(strings.TrimSpace(b.Label), rep.Scene) {
			continue
		}
		label := strings.ToLower(strings.TrimSpace(b.Label))
		if label == "native 30s capture" || label == "presentmon capture" || label == "unlabeled capture" {
			rep.ExcludedCaptures++
			continue
		}
		// Reject blank profile, nonfinite or implausible data. Such records cannot prove a gain.
		if b.Frames < 300 || b.AvgFPS <= 0 || b.Low1 <= 0 || b.P99ms <= 0 ||
			math.IsNaN(b.AvgFPS) || math.IsInf(b.AvgFPS, 0) || math.IsNaN(b.Low1) || math.IsInf(b.Low1, 0) || math.IsNaN(b.P99ms) || math.IsInf(b.P99ms, 0) ||
			strings.TrimSpace(b.Profile) == "" || b.Source == "" {
			rep.ExcludedCaptures++
			continue
		}
		k := strings.ToLower(strings.TrimSpace(b.Profile))
		if _, ok := groups[k]; !ok {
			order = append(order, k)
			display[k] = b.Profile
		}
		groups[k] = append(groups[k], b)
	}
	byName := map[string]ExperimentGroup{}
	for _, key := range order {
		samples := groups[key]
		// Bound aggregation to the latest 25 runs per profile to prevent stale old measurements dominating.
		if len(samples) > 25 {
			samples = samples[:25]
		}
		fps := make([]float64, 0, len(samples))
		low := make([]float64, 0, len(samples))
		p99 := make([]float64, 0, len(samples))
		total := 0
		for _, b := range samples {
			fps = append(fps, b.AvgFPS)
			low = append(low, b.Low1)
			p99 = append(p99, b.P99ms)
			total += b.Frames
		}
		med := medianFloat(p99)
		deviations := make([]float64, 0, len(samples))
		for _, v := range p99 {
			deviations = append(deviations, math.Abs(v-med))
		}
		group := ExperimentGroup{Profile: display[key], Runs: len(samples), Frames: total, MedianFPS: roundPercent(medianFloat(fps)), MedianLow1: roundPercent(medianFloat(low)), MedianP99ms: roundPercent(med), P99MADms: roundPercent(medianFloat(deviations))}
		byName[key] = group
		rep.Groups = append(rep.Groups, group)
	}
	if len(rep.Groups) == 0 {
		rep.Details = "No usable, labeled captures for the selected game and scene. Use the same scene label and a named optimization profile for each run."
		return rep
	}
	bk := strings.ToLower(strings.TrimSpace(before))
	ak := strings.ToLower(strings.TrimSpace(after))
	if bk == "" {
		for _, name := range []string{"stock", "baseline", "default"} {
			if _, ok := byName[name]; ok {
				bk = name
				break
			}
		}
	}
	if ak == "" {
		for _, key := range order {
			if key != bk {
				ak = key
				break
			}
		}
	}
	if bk == "" && len(order) == 2 {
		bk = order[1]
		if ak == "" {
			ak = order[0]
		}
	}
	b, okB := byName[bk]
	a, okA := byName[ak]
	if !okB || !okA || bk == ak {
		rep.Status = "CHOOSE TWO PROFILES"
		rep.Details = "Two distinct profiles are needed for a comparison; specify baseline and candidate explicitly if the automatic selection is not correct."
		return rep
	}
	rep.Baseline = &b
	rep.Candidate = &a
	if b.Runs < 3 || a.Runs < 3 {
		rep.Status = "MORE RUNS REQUIRED"
		rep.Details = "At least three usable captures per profile are required. Sawcha will not label a single-run difference a performance improvement."
		return rep
	}
	rep.AvgFPSDeltaPct = percentChange(b.MedianFPS, a.MedianFPS)
	rep.Low1DeltaPct = percentChange(b.MedianLow1, a.MedianLow1)
	rep.P99DeltaPct = percentChange(b.MedianP99ms, a.MedianP99ms)
	// MAD is a *heuristic variability guard*, not a p-value or a significance test.
	noise := 2 * math.Max(b.P99MADms, a.P99MADms)
	if math.Abs(a.MedianP99ms-b.MedianP99ms) <= noise {
		rep.Status = "INCONCLUSIVE / RUN VARIATION"
		rep.Details = "P99 difference is within twice the observed median absolute deviation; collect more matching runs before judging."
	} else if rep.Low1DeltaPct <= -5 || rep.P99DeltaPct >= 8 {
		rep.Status = "REGRESSION / ROLLBACK CANDIDATE"
		rep.Details = "Repeated captures show worse slower-frame behavior. Restore the previous setting and test again."
	} else if rep.Low1DeltaPct >= 5 && rep.P99DeltaPct <= -5 {
		rep.Status = "PROMISING / VERIFY AGAIN"
		rep.Details = "Median 1% low improved and P99 decreased beyond the simple variability guard. Verify stability and test again; no universal FPS gain is proven."
	} else {
		rep.Status = "INCONCLUSIVE / TRADEOFF"
		rep.Details = "Results do not show a consistent tail-frame improvement. More FPS is not necessarily smoother play."
	}
	return rep
}

func (a *App) experimentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	a.mu.RLock()
	hist := append([]Benchmark(nil), a.history...)
	a.mu.RUnlock()
	q := r.URL.Query()
	send(w, buildExperimentReport(hist, q.Get("game"), q.Get("scene"), q.Get("baseline"), q.Get("candidate")))
}