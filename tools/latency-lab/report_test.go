package main

import (
	"strings"
	"testing"
)

func TestNoAssumptionsWithoutReport(t *testing.T) {
	if text := offlineAdvice("FPS", nil); !strings.Contains(text, "Run a diagnostic scan") {
		t.Fatal(text)
	}
}
func TestAssessmentUsesMeasuredCPUAndMemory(t *testing.T) {
	report := Report{WindowsData: map[string]any{
		"cpu": map[string]any{"LoadPercentage": 92.0},
		"os":  map[string]any{"TotalVisibleMemorySize": 32000000.0, "FreePhysicalMemory": 2000000.0},
	}}
	tips := strings.Join(assessment(report), "\n")
	if !strings.Contains(tips, "CPU load exceeds") || !strings.Contains(tips, "Less than 12%") {
		t.Fatal(tips)
	}
}
