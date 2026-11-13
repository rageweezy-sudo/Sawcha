package main

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// Analysis is deliberately read-only: it never writes a driver, flashes a VBIOS,
// changes power limits, injects into a game, or disables Windows security.
type InsightAction struct {
	Priority     int    `json:"priority"`
	Category     string `json:"category"`
	Title        string `json:"title"`
	Why          string `json:"why"`
	How          string `json:"how"`
	Risk         string `json:"risk"`
	Verification string `json:"verification"`
}

type BenchmarkComparison struct {
	Game            string  `json:"game"`
	Before          string  `json:"before"`
	After           string  `json:"after"`
	AvgFPSChangePct float64 `json:"avgFPSChangePct"`
	Low1ChangePct   float64 `json:"low1ChangePct"`
	P99msChangePct  float64 `json:"p99msChangePct"`
	Assessment      string  `json:"assessment"`
	Caveat          string  `json:"caveat"`
}

type InsightsReport struct {
	Game        string               `json:"game"`
	Status      string               `json:"status"`
	FrameSample *Benchmark           `json:"frameSample,omitempty"`
	Bottleneck  string               `json:"bottleneck"`
	Explanation string               `json:"explanation"`
	Actions     []InsightAction      `json:"actions"`
	Comparison  *BenchmarkComparison `json:"comparison,omitempty"`
	Method      string               `json:"method"`
}

func roundPercent(v float64) float64 { return math.Round(v*10) / 10 }
func percentChange(before, after float64) float64 {
	if before <= 0 || math.IsNaN(before) || math.IsInf(before, 0) {
		return 0
	}
	return roundPercent((after/before - 1) * 100)
}

func compareRuns(before, after Benchmark) *BenchmarkComparison {
	if before.Frames < 300 || after.Frames < 300 || before.AvgFPS <= 0 || after.AvgFPS <= 0 || before.Low1 <= 0 || after.Low1 <= 0 || before.P99ms <= 0 || after.P99ms <= 0 {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(before.Game), strings.TrimSpace(after.Game)) ||
		!strings.EqualFold(strings.TrimSpace(before.Label), strings.TrimSpace(after.Label)) ||
		strings.TrimSpace(before.Label) == "" {
		return nil
	}
	comp := &BenchmarkComparison{Game: after.Game, Before: before.Profile, After: after.Profile,
		AvgFPSChangePct: percentChange(before.AvgFPS, after.AvgFPS),
		Low1ChangePct:   percentChange(before.Low1, after.Low1),
		P99msChangePct:  percentChange(before.P99ms, after.P99ms),
		Caveat:          "One capture per condition is only directional. Repeat the SAME scene, resolution, graphics settings and capture length at least three times per condition; compare medians and system temperatures."}
	switch {
	case strings.EqualFold(before.Profile, after.Profile):
		comp.Assessment = "REPEAT BASELINE: both captures used the same profile; differences are natural variability until the scene is controlled"
	case comp.Low1ChangePct <= -5 || comp.P99msChangePct >= 8:
		comp.Assessment = "REGRESSION: slower tail frames; roll back the tweak until repeat runs prove otherwise"
	case comp.Low1ChangePct >= 5 && comp.P99msChangePct <= -5:
		comp.Assessment = "PROMISING: improved slower frames in this capture; repeat before keeping the change"
	case comp.AvgFPSChangePct >= 5 && comp.Low1ChangePct < -2:
		comp.Assessment = "MIXED: average FPS rose but slower frames became worse"
	default:
		comp.Assessment = "INCONCLUSIVE: differences are small, mixed, or could be run-to-run noise"
	}
	return comp
}

func chooseBenchmarks(history []Benchmark, game string) (*Benchmark, *BenchmarkComparison) {
	selected := []Benchmark{}
	for _, b := range history {
		if strings.EqualFold(strings.TrimSpace(b.Game), strings.TrimSpace(game)) {
			selected = append(selected, b)
		}
	}
	if len(selected) == 0 {
		return nil, nil
	}
	latest := selected[0]
	for _, previous := range selected[1:] {
		if c := compareRuns(previous, latest); c != nil {
			return &latest, c
		}
	}
	return &latest, nil
}

func buildInsights(hardware map[string]interface{}, history []Benchmark, games []Game, preferredGame string) InsightsReport {
	game := strings.TrimSpace(preferredGame)
	if game == "" && len(history) > 0 {
		game = history[0].Game
	}
	if game == "" && len(games) > 0 {
		game = games[0].Name
	}
	if game == "" {
		game = "Not selected"
	}
	report := InsightsReport{Game: game, Status: "Baseline needed", Bottleneck: "Not measured", Explanation: "Run a PresentMon 2.6+ capture while playing a repeatable scene. Sawcha does not infer a bottleneck from the GPU's marketing name alone.", Method: "Read-only analysis. Run the same game scene three times before and after one reversible change; never assume a faster FPS counter means fewer stutters."}
	add := func(pri int, cat, title, why, how, risk, check string) {
		report.Actions = append(report.Actions, InsightAction{pri, cat, title, why, how, risk, check})
	}
	sample, comp := chooseBenchmarks(history, game)
	report.FrameSample, report.Comparison = sample, comp
	if sample != nil {
		if sample.Frames < 500 {
			report.Status = "Short capture: collect more frames"
		} else {
			report.Status = "Frame capture available"
		}
		report.Explanation = fmt.Sprintf("Latest sample: %.1f average FPS, %.1f FPS 1%% low, %.1f ms P99, %.1f%% large frame spikes (%d app frames). These reflect one capture, not a guaranteed performance improvement.", sample.AvgFPS, sample.Low1, sample.P99ms, sample.StutterPct, sample.Frames)
		switch {
		case sample.GPUBusyAvgms > 0 && sample.AvgFPS > 0 && sample.GPUBusyAvgms >= 0.83*(1000/sample.AvgFPS):
			report.Bottleneck = "Likely GPU-heavy in measured scene"
			add(1, "Graphics", "Reduce GPU-heavy graphics first", "GPU busy time is close to the time between presented frames; graphics settings are a plausible limiter.", "Lower shadows, volumetric effects or render resolution one step; keep a saved baseline. For NVIDIA games supporting Reflex, try the game's built-in Reflex setting.", "Less visual detail; dynamic resolution can blur text.", "Repeat the same frame capture and compare P99, 1% lows and image quality.")
		case sample.GPUBusyAvgms > 0 && sample.AvgFPS > 0 && sample.GPUBusyAvgms <= 0.52*(1000/sample.AvgFPS):
			report.Bottleneck = "GPU not continuously busy; investigate CPU, frame cap or waiting"
			add(1, "Frame pacing", "Investigate CPU / game-thread / frame cap", "GPU busy time is much shorter than frame intervals. This alone does not prove a CPU bottleneck.", "Compare per-core CPU activity, game frame cap, recording overlays and background tasks. Keep security protections enabled.", "Disabling unrelated services can cause instability and usually doesn't help.", "Look for stable 1% lows and lower P99 in repeatable scenes.")
		default:
			report.Bottleneck = "Not enough GPU-busy data to identify the limiting stage"
			add(2, "Measurement", "Capture GPU busy time and frame pacing", "Average FPS alone cannot distinguish a busy GPU from game-thread stalls or external waits.", "Use current PresentMon with GPU tracking on a consistent scene.", "Capture software has some overhead.", "Confirm MsGPUBusy and MsBetweenPresents appear in the CSV.")
		}
		if sample.P99ms > 0 && sample.Medianms > 0 && sample.P99ms > 1.8*sample.Medianms {
			add(1, "Stutter", "Investigate periodic frame spikes", "The slowest frames take much longer than the median in this sample.", "Check shader-compilation events if your capture supports them, loading areas, VRAM pressure, streaming/recording, and disk access. Test after the game's shader cache warms up.", "Repeated scene loading can make comparisons inconsistent.", "Repeat runs and compare P99 and stutter percentage.")
		}
		if sample.GeneratedFrames > 0 {
			add(1, "Frame generation", "Distinguish rendered FPS from generated FPS", "The capture included driver- or SDK-generated frames, which can make displayed FPS differ from rendered game FPS.", "Use application-frame metrics for CPU/GPU performance and separately check displayed FPS and measured input latency.", "Frame generation may increase display smoothness without speeding up simulation or reducing input delay.", "Confirm FrameType is recorded and compare app vs displayed frames consistently.")
		}
		if comp != nil && strings.HasPrefix(comp.Assessment, "REGRESSION") {
			add(1, "Rollback", "Revert the last optimization until retested", "This run had noticeably worse slower-frame behavior than its matching baseline.", "Use Sawcha Restore if the change used Sawcha presets; otherwise restore the original game setting manually.", "Restoring a setting while playing can cause interruption.", "Re-test with three identical runs.")
		}
	} else {
		add(1, "Baseline", "Capture a baseline before changing Windows", "Without repeatable frame-time data, an optimization can feel faster while actually causing more stutters.", "Open your game, capture 30–120 seconds in the same scene, and save the results before applying one change.", "PresentMon may need installation and has minor capture overhead.", "Save 3 matching baseline captures before doing A/B comparisons.")
	}
	nvidia := asMaps(hardware["nvidia"])
	if len(nvidia) > 0 {
		vram := nvidia[0]
		total := toFloat(vram["vramTotalMB"])
		used := toFloat(vram["vramUsedMB"])
		if total > 0 && used/total >= 0.89 {
			add(1, "VRAM", "Reduce texture memory pressure", "NVIDIA reports graphics memory nearly full at the instant of the scan. This isn't proof of game-specific VRAM thrashing.", "Reduce texture quality and close unneeded GPU workloads; capture again under the same gameplay load.", "Texture quality can look worse at distance.", "Check whether GPU memory stays below its practical limit and P99 spikes fall.")
		}
	}
	for _, d := range asMaps(hardware["Disks"]) {
		size, free := toFloat(d["Size"]), toFloat(d["FreeSpace"])
		if size > 0 && free/size < .12 {
			add(2, "Storage", "Free space on the game drive", "Very low free space can interfere with updates, recordings, caches or paging.", "Use Windows Storage settings to remove known unnecessary files; do not blindly delete cache folders.", "Deleting saved game or application files is irreversible without backup.", "Re-check disk free space and repeat the same benchmark.")
		}
	}
	add(3, "Windows", "Check Game Mode and per-game graphics preference", "Modern Windows has supported graphics controls that may improve compatibility without registry hacks.", "Open Windows Settings > Gaming > Game Mode and System > Display > Graphics; compare settings one at a time. On Windows 11, consider Optimizations for windowed games when relevant.", "Some window modes or game integrations behave differently; restart the game after changes.", "Use repeatable game captures, not the Windows desktop FPS.")
	sort.SliceStable(report.Actions, func(i, j int) bool { return report.Actions[i].Priority < report.Actions[j].Priority })
	return report
}

type GPUComputeReport struct {
	Device         string   `json:"device"`
	Family         string   `json:"family"`
	Workload       string   `json:"workload"`
	ResearchStatus string   `json:"researchStatus"`
	GameFPSBenefit string   `json:"gameFPSBenefit"`
	WindowsSupport string   `json:"windowsSupport"`
	SafeNextSteps  []string `json:"safeNextSteps"`
	ProjectURL     string   `json:"projectURL"`
}

var rtxModelRx = regexp.MustCompile(`(?i)\bRTX\s*(20|30|40|50)\d{2}\b`)

func assessGPUCompute(hardware map[string]interface{}) GPUComputeReport {
	report := GPUComputeReport{Device: "No NVIDIA device detected", Family: "Unknown", Workload: "AI training, unquantized inference, diffusion image/video generation", ResearchStatus: "Not assessed", GameFPSBenefit: "No proven improvement to gaming FPS", WindowsSupport: "Sawcha does not install or patch kernel drivers or GPU firmware.", ProjectURL: "https://github.com/Zagitta/gpu-unlock", SafeNextSteps: []string{
		"Use vendor drivers and application-supported CUDA compute settings.",
		"Benchmark the actual AI workload before/after any vendor-supported configuration changes.",
		"For gaming, use the frame-time lab instead; tensor-throughput unlocks do not raise ordinary game FPS.",
	}}
	names := []string{}
	for _, g := range asMaps(hardware["nvidia"]) {
		if s := strings.TrimSpace(fmt.Sprint(g["name"])); s != "" && s != "<nil>" {
			names = append(names, s)
		}
	}
	if len(names) == 0 {
		for _, g := range asMaps(hardware["GPU"]) {
			s := fmt.Sprint(g["Name"])
			if strings.Contains(strings.ToLower(s), "nvidia") {
				names = append(names, s)
			}
		}
	}
	if len(names) == 0 {
		report.Family = "Not NVIDIA / unavailable"
		report.ResearchStatus = "Not applicable"
		return report
	}
	report.Device = names[0]
	match := rtxModelRx.FindStringSubmatch(report.Device)
	if len(match) < 2 {
		report.Family = "Other NVIDIA"
		report.ResearchStatus = "Unverified hardware; not an unlock recommendation"
		return report
	}
	report.Family = "GeForce RTX " + match[1] + " series"
	switch match[1] {
	case "30":
		report.ResearchStatus = "Linux-only upstream reports stable support on selected models; check the exact PCI device ID"
	case "20", "40":
		report.ResearchStatus = "Linux upstream lists experimental support; exact device coverage matters"
	case "50":
		report.ResearchStatus = "Zagitta upstream does not support RTX 50 / Blackwell"
	}
	report.WindowsSupport = "Original Zagitta gpu-unlock project is Linux-only. Unofficial Windows ports involve boot/firmware/driver changes and are not integrated into Sawcha. These are not FPS optimizations."
	return report
}
