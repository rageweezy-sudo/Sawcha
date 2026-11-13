package main

import (
 "errors"
 "fmt"
 "math"
 "net/http"
 "strings"
)

// LatencyComparison compares PresentMon frame pacing only. It does not
// measure mouse-to-photon, USB input, DPC/ISR, network RTT, or audio latency.
// A comparison requires repeat captures of the same game and scene.
type LatencyComparison struct {
 Status string `json:"status"`
 BeforeGame string `json:"game"`
 BaselineP99MS float64 `json:"baselineP99Ms"`
 AfterP99MS float64 `json:"afterP99Ms"`
 P99ChangePercent float64 `json:"p99ChangePercent"`
 MedianChangePercent float64 `json:"medianChangePercent"`
 Low1ChangePercent float64 `json:"low1ChangePercent"`
 RollbackRecommended bool `json:"rollbackRecommended"`
 Message string `json:"message"`
}

func percentageChange(before,after float64) float64 {
 return 100*(after-before)/before
}
func compareLatency(before,after Benchmark) LatencyComparison {
 out:=LatencyComparison{Status:"inconclusive",BeforeGame:before.Game,
   BaselineP99MS:before.P99ms,AfterP99MS:after.P99ms,
   Message:"Measurements cannot establish that a change is safe. Keep the original settings or collect paired captures of the same scene."}
 if before.ID==after.ID||strings.TrimSpace(before.Game)==""||!strings.EqualFold(strings.TrimSpace(before.Game),strings.TrimSpace(after.Game))||
    strings.TrimSpace(before.Label)==""||!strings.EqualFold(strings.TrimSpace(before.Label),strings.TrimSpace(after.Label))||
    before.Frames<300||after.Frames<300||before.P99ms<=0||after.P99ms<=0||
    before.Medianms<=0||after.Medianms<=0||before.Low1<=0||after.Low1<=0 ||
    math.IsNaN(before.P99ms)||math.IsNaN(after.P99ms)||math.IsInf(before.P99ms,0)||math.IsInf(after.P99ms,0) {
    return out
 }
 out.P99ChangePercent=percentageChange(before.P99ms,after.P99ms)
 out.MedianChangePercent=percentageChange(before.Medianms,after.Medianms)
 out.Low1ChangePercent=percentageChange(before.Low1,after.Low1)
 // 3% is a *conservative flagging threshold*, not statistical proof.
 // Apply the rule to P99 and median, and only flag significant Low1 decreases.
 if out.P99ChangePercent>3||out.MedianChangePercent>3||out.Low1ChangePercent < -5 {
    out.Status="regressed"
    out.RollbackRecommended=true
    out.Message=fmt.Sprintf("Possible frame-pacing regression (P99 %+.1f%%, median %+.1f%%, 1%%-low FPS %+.1f%%). Restore the previous SAWCHA setting and repeat the same-scene test. Other latencies have NOT been measured.",out.P99ChangePercent,out.MedianChangePercent,out.Low1ChangePercent)
 } else if out.P99ChangePercent < -3 && out.MedianChangePercent<=3 && out.Low1ChangePercent>=-5 {
    out.Status="improved"
    out.Message=fmt.Sprintf("P99 frame time improved %.1f%% in this sample. This does NOT prove lower input, audio or network latency; repeat trials to confirm.",-out.P99ChangePercent)
 } else {
    out.Status="no_clear_change"
    out.Message="No clear frame-time regression detected in this single comparison. Results may vary; other latency paths have not been measured."
 }
 return out
}

func (a *App) latencyCompareHandler(w http.ResponseWriter,r *http.Request) {
 var request struct {
  BeforeID string `json:"beforeID"`
  AfterID string `json:"afterID"`
 }
 if err:=decode(r,&request);err!=nil{bad(w,err.Error(),400);return}
 if request.BeforeID==""||request.AfterID==""||request.BeforeID==request.AfterID{
    bad(w,"Two distinct saved benchmark IDs are required",400);return
 }
 a.mu.RLock()
 var before,after Benchmark
 haveBefore,haveAfter:=false,false
 for _,b:=range a.history {
   if b.ID==request.BeforeID {before=b;haveBefore=true}
   if b.ID==request.AfterID {after=b;haveAfter=true}
 }
 a.mu.RUnlock()
 if !haveBefore||!haveAfter {bad(w,"Saved before/after benchmark not found",404);return}
 send(w,compareLatency(before,after))
}

// Strict latency mode is an opt-out protection. It blocks speculative
// registry and power-scheme modifications, including even reversible
// optimization presets. Read-only scans and restore actions remain allowed.
func (a *App) speculativeSystemChangesAllowed() error {
 a.mu.RLock()
 safe:=a.preferences.LatencySafe
 a.mu.RUnlock()
 if safe {
  return errors.New("Strict latency protection is ON: unmeasured Windows registry/power changes are blocked. The original settings remain unchanged. To experiment, switch OFF Strict Latency Protection in Settings, record a baseline first, and restore changes if frame pacing regresses.")
 }
 return nil
}
func shouldSkipHeavyMonitor(game string,prefs AppPreferences) bool {
 return game!="" && prefs.DetectGames && (prefs.LatencySafe||prefs.QuietWhileGaming)
}

func shouldRunLiveMonitor(recentlyViewed bool,game string,prefs AppPreferences) bool {
 if shouldSkipHeavyMonitor(game,prefs){return false}
 // Strict mode is idle without background PowerShell/WMI collection.
 return !prefs.LatencySafe||recentlyViewed
}
