package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func sampleCondition(p string, n int, fps, low, p99 float64) []Benchmark {
	out := make([]Benchmark, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, Benchmark{Game: "Rainbow Six Siege", Label: "Same training hall", Profile: p, Source: "PresentMon", Frames: 3000, AvgFPS: fps, Low1: low, P99ms: p99})
	}
	return out
}

func TestExperimentRequiresThreeRunsAndSameScene(t *testing.T) {
	hist := append(sampleCondition("Stock", 3, 100, 60, 25), sampleCondition("Competitive", 1, 115, 70, 20)...)
	report := buildExperimentReport(hist, "Rainbow Six Siege", "Same training hall", "Stock", "Competitive")
	if report.Status != "MORE RUNS REQUIRED" {
		t.Fatalf("false confidence: %+v", report)
	}
	hist = append(hist, sampleCondition("Competitive", 2, 115, 70, 20)...)
	report = buildExperimentReport(hist, "Rainbow Six Siege", "Different scene", "Stock", "Competitive")
	if report.Baseline != nil || report.Candidate != nil {
		t.Fatalf("cross-scene comparison: %+v", report)
	}
}
func TestRepeatedCapturesPromising(t *testing.T) {
	hist := append(sampleCondition("Competitive", 3, 116, 75, 18), sampleCondition("Stock", 3, 100, 60, 25)...)
	report := buildExperimentReport(hist, "Rainbow Six Siege", "Same training hall", "Stock", "Competitive")
	if report.Status != "PROMISING / VERIFY AGAIN" || report.Low1DeltaPct < 20 || report.P99DeltaPct > -20 {
		t.Fatalf("expected promising result, got %+v", report)
	}
}
func TestExperimentRejectsNoisyChange(t *testing.T) {
	before := sampleCondition("Stock", 3, 100, 60, 25)
	before[0].P99ms = 22
	before[2].P99ms = 28
	after := sampleCondition("Optimized", 3, 106, 65, 23)
	rep := buildExperimentReport(append(after, before...), "Rainbow Six Siege", "Same training hall", "Stock", "Optimized")
	if rep.Status != "INCONCLUSIVE / RUN VARIATION" {
		t.Fatalf("must not claim noise as a win: %+v", rep)
	}
}
func TestExperimentDetectsStutterRegressionDespiteHigherAverage(t *testing.T) {
	hist := append(sampleCondition("Optimized", 3, 140, 48, 34), sampleCondition("Stock", 3, 125, 75, 22)...)
	rep := buildExperimentReport(hist, "Rainbow Six Siege", "Same training hall", "Stock", "Optimized")
	if rep.Status != "REGRESSION / ROLLBACK CANDIDATE" || rep.AvgFPSDeltaPct <= 0 {
		t.Fatalf("missed tail regression: %+v", rep)
	}
}
func TestExperimentExcludedInvalidAndProfileSeparation(t *testing.T) {
	hist := append(sampleCondition("Stock", 3, 100, 60, 25), sampleCondition("Optimized", 3, 112, 70, 19)...)
	bad := sampleCondition("Optimized", 1, 999, 999, 0)[0]
	hist = append([]Benchmark{bad}, hist...)
	rep := buildExperimentReport(hist, "Rainbow Six Siege", "Same training hall", "Stock", "Optimized")
	if rep.ExcludedCaptures != 1 || rep.Candidate.Runs != 3 {
		t.Fatalf("invalid run was included: %+v", rep)
	}
	rep = buildExperimentReport(hist, "Rainbow Six Siege", "Same training hall", "Stock", "Stock")
	if rep.Status != "CHOOSE TWO PROFILES" {
		t.Fatalf("same-profile falsely compared: %+v", rep)
	}
}
func TestExperimentsEndpointAuthenticationAndMethod(t *testing.T) {
	a := &App{token: "secret", history: sampleCondition("Stock", 3, 100, 60, 25), hardware: map[string]interface{}{}}
	a.initAuth()
	s := httptest.NewServer(a.routes())
	defer s.Close()
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req, _ := http.NewRequest(method, s.URL+"/api/experiments", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("unauthenticated %s returned %d", method, resp.StatusCode)
		}
	}
	rr := httptest.NewRecorder()
	a.experimentHandler(rr, httptest.NewRequest(http.MethodGet, "/api/experiments?game="+"Rainbow%20Six%20Siege", nil))
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Code)
	}
	var r ExperimentReport
	if err := json.Unmarshal(rr.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Groups) != 1 {
		t.Fatal(fmt.Sprint(r.Groups))
	}
}
func TestPreferAppRenderIntervalsToDisplayIntervals(t *testing.T) {
	s := &strings.Builder{}
	fmt.Fprintln(s, "Application,MsBetweenPresents,MsBetweenAppPresents,FrameType")
	for i := 0; i < 400; i++ {
		fmt.Fprintln(s, "game.exe,8.333,16.666,Application")
	}
	b, e := parseBenchmark(strings.NewReader(s.String()))
	if e != nil || b.AvgFPS < 59 || b.AvgFPS > 61 {
		t.Fatalf("misleading generated display rate: b=%+v e=%v", b, e)
	}
}

func TestExperimentRejectsGenericUncontrolledScene(t *testing.T) {
	history := sampleCondition("Stock", 3, 100, 60, 25)
	for i := range history {
		history[i].Label = "Native 30s capture"
	}
	r := buildExperimentReport(history, "Rainbow Six Siege", "", "", "")
	if len(r.Groups) != 0 || r.ExcludedCaptures != 3 {
		t.Fatalf("generic scene should not be trusted: %+v", r)
	}
}