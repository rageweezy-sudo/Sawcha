package main

import (
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

func testLatencyBenchmark(id, game, label string, p99, median, low1 float64) Benchmark {
 return Benchmark{ID:id,Game:game,Label:label,Frames:1800,P99ms:p99,Medianms:median,Low1:low1}
}
func TestLatencyGuardComparison(t *testing.T) {
 before:=testLatencyBenchmark("before","Rainbow Six Siege","same training scene",23,16,48)
 better:=testLatencyBenchmark("after","Rainbow Six Siege","same training scene",20,15,52)
 report:=compareLatency(before,better)
 if report.Status!="improved" {t.Fatalf("improved case: %+v",report)}
 worse:=testLatencyBenchmark("after","Rainbow Six Siege","same training scene",26,17,44)
 report=compareLatency(before,worse)
 if report.Status!="regressed"||!report.RollbackRecommended{t.Fatalf("regression must recommend restoring: %+v",report)}
 nearly:=testLatencyBenchmark("after","Rainbow Six Siege","same training scene",23.1,16.1,47.8)
 report=compareLatency(before,nearly)
 if report.Status!="no_clear_change"{t.Fatalf("noise must be inconclusive as an improvement: %+v",report)}
}
func TestLatencyGuardRejectsUncomparableMeasurements(t *testing.T) {
 before:=testLatencyBenchmark("x","Siege","test scene",20,15,50)
 after:=testLatencyBenchmark("y","Fortnite","test scene",18,14,53)
 if result:=compareLatency(before,after);result.Status!="inconclusive"{t.Fatalf("wrong game %+v",result)}
 after.Game="Siege";after.Label="different scene"
 if result:=compareLatency(before,after);result.Status!="inconclusive"{t.Fatalf("scene mismatch %+v",result)}
 after.Label=before.Label;after.Frames=50
 if result:=compareLatency(before,after);result.Status!="inconclusive"{t.Fatalf("insufficient samples %+v",result)}
 after.Frames=1800;after.P99ms=0
 if result:=compareLatency(before,after);result.Status!="inconclusive"{t.Fatalf("invalid measurement %+v",result)}
}
func TestSafeModeBlocksSpeculativeOSChanges(t *testing.T) {
 app:=&App{root:t.TempDir(),preferences:defaultAppPreferences()}
 if !app.preferences.LatencySafe{t.Fatal("strict safe mode must be default")}
 req:=httptest.NewRequest("POST","/api/preset",strings.NewReader(`{"profile":"Competitive","power":false}`))
 rec:=httptest.NewRecorder()
 app.presetHandler(rec,req)
 if rec.Code!=http.StatusConflict{t.Fatalf("unsafe preset not blocked: %d %s",rec.Code,rec.Body.String())}
 if app.config.Backup!=nil{t.Fatal("blocked preset created a backup or mutated state")}
}
func TestLatencyComparisonAPIReadOnlyAndValidatesIDs(t *testing.T) {
 app:=&App{history:[]Benchmark{
  testLatencyBenchmark("new","Test Game","same scene",23,17,49),
  testLatencyBenchmark("old","Test Game","same scene",20,16,52),
 }}
 req:=httptest.NewRequest("POST","/api/latency/compare",strings.NewReader(`{"beforeID":"old","afterID":"new"}`))
 rec:=httptest.NewRecorder()
 app.latencyCompareHandler(rec,req)
 if rec.Code!=http.StatusOK||!strings.Contains(rec.Body.String(),"regressed"){t.Fatalf("comparison API: %d %s",rec.Code,rec.Body.String())}
 req=httptest.NewRequest("POST","/api/latency/compare",strings.NewReader(`{"beforeID":"unknown","afterID":"new"}`))
 rec=httptest.NewRecorder()
 app.latencyCompareHandler(rec,req)
 if rec.Code!=http.StatusNotFound{t.Fatalf("unknown benchmark must not be compared: %d",rec.Code)}
}
func TestStrictModeSkipsHeavyMonitoring(t *testing.T) {
 prefs:=defaultAppPreferences()
 if !shouldSkipHeavyMonitor("Siege",prefs) {t.Fatal("in-game WMI should be paused")}
 if shouldSkipHeavyMonitor("",prefs) {t.Fatal("idle PC should allow monitoring")}
 prefs.LatencySafe=false;prefs.QuietWhileGaming=false
 if shouldSkipHeavyMonitor("Siege",prefs) {t.Fatal("explicit opt-out should return to regular monitoring")}
}

func TestStrictModeBlocksPointerSpeedChanges(t *testing.T) {
 app:=&App{preferences:defaultAppPreferences()}
 req:=httptest.NewRequest("POST","/api/mouse",strings.NewReader(`{"speed":10}`))
 rec:=httptest.NewRecorder()
 app.mouseHandler(rec,req)
 if rec.Code!=http.StatusConflict {t.Fatalf("mouse settings changed in latency-safe mode: %d %s",rec.Code,rec.Body.String())}
}

func TestStrictModeRunsHeavyTelemetryOnlyWhenLiveMonitorViewed(t *testing.T) {
 prefs:=defaultAppPreferences()
 if shouldRunLiveMonitor(false,"",prefs) {t.Fatal("background WMI must be off in strict mode")}
 if !shouldRunLiveMonitor(true,"",prefs) {t.Fatal("explicit live view must be allowed")}
 if shouldRunLiveMonitor(true,"Siege",prefs) {t.Fatal("live WMI must pause for a detected game")}
 prefs.LatencySafe=false
 if !shouldRunLiveMonitor(false,"",prefs) {t.Fatal("opt-out may use standard background live telemetry")}
}
