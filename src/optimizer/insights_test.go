package main

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPresentMon26FiltersAppAndGeneratedFrames(t *testing.T) {
	rows := &strings.Builder{}
	fmt.Fprintln(rows, "Application,MsBetweenPresents,FrameType,MsGPUBusy,MsCPUBusy")
	for i := 0; i < 600; i++ {
		fmt.Fprintln(rows, "rainbowsix.exe,16.666,Application,15.0,4.0")
		if i%10 == 0 {
			fmt.Fprintln(rows, "rainbowsix.exe,8.33,Generated,0,0")
		}
		fmt.Fprintln(rows, "obs64.exe,33.33,Application,29,20")
	}
	got, err := parseBenchmarkForProcess(strings.NewReader(rows.String()), "RainbowSix.exe")
	if err != nil {
		t.Fatal(err)
	}
	if got.Frames != 600 || got.GeneratedFrames != 60 {
		t.Fatalf("filtered frame count wrong: %+v", got)
	}
	if math.Abs(got.GPUBusyAvgms-15) > 0.001 || math.Abs(got.CPUBusyAvgms-4) > 0.001 {
		t.Fatalf("wrong GPU/CPU busy metrics: %+v", got)
	}
	if got.AvgFPS < 59.8 || got.AvgFPS > 60.2 || got.P95ms < 16.5 {
		t.Fatalf("unexpected frame metrics: %+v", got)
	}
}

func TestPresentMonRejectsMixedApplications(t *testing.T) {
	rows := &strings.Builder{}
	fmt.Fprintln(rows, "Application,MsBetweenPresents")
	for i := 0; i < 50; i++ {
		fmt.Fprintln(rows, "game.exe,16.6")
		fmt.Fprintln(rows, "obs.exe,12.0")
	}
	_, err := parseBenchmark(strings.NewReader(rows.String()))
	if err == nil || !strings.Contains(err.Error(), "multiple application") {
		t.Fatalf("mixed capture should be rejected: %v", err)
	}
}

func TestPresentMonSingleGameNoGPUDataStillUseful(t *testing.T) {
	rows := &strings.Builder{}
	fmt.Fprintln(rows, "Application,MsBetweenPresents")
	for i := 0; i < 400; i++ {
		fmt.Fprintln(rows, "game.exe,12.5")
	}
	b, err := parseBenchmark(strings.NewReader(rows.String()))
	if err != nil || b.GPUBusyAvgms != 0 || b.Frames != 400 {
		t.Fatalf("legacy CSV unavailable: %+v %v", b, err)
	}
}

func goodRun(profile string) Benchmark {
	return Benchmark{Game: "Rainbow Six Siege", Label: "Custom hall camera", Profile: profile, Frames: 2000, AvgFPS: 120, Low1: 79, P99ms: 24, Medianms: 8.3, StutterPct: 1.3, GPUBusyAvgms: 7.6}
}
func TestABComparisonKeepsFramePacingHonest(t *testing.T) {
	old := goodRun("Stock")
	newer := goodRun("Competitive")
	newer.AvgFPS = 134
	newer.Low1 = 65
	newer.P99ms = 32
	c := compareRuns(old, newer)
	if c == nil || !strings.HasPrefix(c.Assessment, "REGRESSION") || c.AvgFPSChangePct <= 0 {
		t.Fatalf("must warn when 1%% lows worsen despite high average: %+v", c)
	}
	other := newer
	other.Label = "Completely different map"
	if compareRuns(old, other) != nil {
		t.Fatal("different scenes are not comparable")
	}
	other = newer
	other.Frames = 40
	if compareRuns(old, other) != nil {
		t.Fatal("tiny samples should be rejected")
	}
}
func TestSamePresetIsNotPromotedAsOptimization(t *testing.T) {
	a := goodRun("Stock")
	b := goodRun("Stock")
	b.Low1 = 98
	b.P99ms = 18
	c := compareRuns(a, b)
	if c == nil || !strings.HasPrefix(c.Assessment, "REPEAT BASELINE") {
		t.Fatalf("same settings cannot be reported as a tweak benefit: %+v", c)
	}
}

func TestNextgenActionsRiskAndVerification(t *testing.T) {
	hw := map[string]interface{}{"nvidia": []map[string]interface{}{{"name": "NVIDIA GeForce RTX 3050", "vramUsedMB": 3500.0, "vramTotalMB": 4096.0}}}
	g := goodRun("Stock")
	report := buildInsights(hw, []Benchmark{g}, []Game{{Name: "Rainbow Six Siege"}}, "Rainbow Six Siege")
	if !strings.Contains(report.Bottleneck, "GPU-heavy") {
		t.Fatalf("incorrect bottleneck: %+v", report)
	}
	if len(report.Actions) < 2 {
		t.Fatalf("recommendations missing: %+v", report)
	}
	for _, a := range report.Actions {
		if a.Risk == "" || a.Verification == "" || a.How == "" {
			t.Fatalf("unexplained action: %+v", a)
		}
	}
}
func TestComputeResearchDoesNotClaimGamingBoost(t *testing.T) {
	for _, v := range []struct{ model, want string }{{"NVIDIA GeForce RTX 3050", "RTX 30"}, {"GeForce RTX 4080", "RTX 40"}, {"NVIDIA GeForce RTX 5080", "RTX 50"}, {"NVIDIA GeForce GTX 1080", "Other"}} {
		a := assessGPUCompute(map[string]interface{}{"nvidia": []map[string]interface{}{{"name": v.model}}})
		if !strings.Contains(a.Family, v.want) {
			t.Errorf("%s got %q", v.model, a.Family)
		}
		if !strings.Contains(strings.ToLower(a.GameFPSBenefit), "no proven") {
			t.Fatal("GPU unlock cannot promise game FPS gain")
		}
		if strings.Contains(strings.ToLower(a.WindowsSupport), "automatic") {
			t.Fatalf("false Windows support claim: %+v", a)
		}
	}
}
func TestNextgenEndpointsRequireLocalSession(t *testing.T) {
	a := &App{token: "unit-secret", hardware: map[string]interface{}{}, config: SavedConfig{Preset: "Stock"}}
	a.initAuth()
	srv := httptest.NewServer(a.routes())
	defer srv.Close()
	for _, url := range []string{"/api/insights", "/api/gpu/compute"} {
		req, _ := http.NewRequest("GET", srv.URL+url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("untrusted request to %s was allowed: %d", url, resp.StatusCode)
		}
	}
}
