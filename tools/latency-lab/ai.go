package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const modelID = "gpt-6-luna"

type chatTurn struct {
	Role string `json:"role"`
	Text string `json:"text"`
}
type chatInput struct {
	Question      string     `json:"question"`
	ApiKey        string     `json:"apiKey"`
	IncludeReport bool       `json:"includeReport"`
	History       []chatTurn `json:"history"`
}

func (s *server) chatHandler(w http.ResponseWriter, r *http.Request) {
	var input chatInput
	if err := decodeJSON(r, &input, 35000); err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	if err := validateChat(input); err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	s.mu.RLock()
	report := s.latest
	s.mu.RUnlock()
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	reply, err := askAI(ctx, s.client, input, report)
	if err != nil {
		errorJSON(w, 502, err.Error())
		return
	}
	sendJSON(w, map[string]any{"answer": reply, "model": modelID, "reportShared": input.IncludeReport && report != nil})
}
func validateChat(x chatInput) error {
	if l := len(strings.TrimSpace(x.Question)); l < 1 || l > 1800 {
		return errors.New("Question must be 1-1800 characters")
	}
	if len(x.ApiKey) < 20 || len(x.ApiKey) > 600 || strings.ContainsAny(x.ApiKey, "\r\n ") {
		return errors.New("Enter a valid OpenAI API key in the password field")
	}
	if len(x.History) > 8 {
		return errors.New("Chat history is limited to 8 messages")
	}
	for _, t := range x.History {
		if t.Role != "user" && t.Role != "assistant" {
			return errors.New("Invalid conversation role")
		}
		if len(t.Text) > 1800 {
			return errors.New("Chat message too long")
		}
	}
	return nil
}
func askAI(ctx context.Context, client *http.Client, x chatInput, report *Report) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second}
	}
	instructions := "You are SAWCHA Latency Lab's technical diagnostic assistant. The user's data is a local snapshot, not live direct access. Explain likely bottlenecks and safe, testable optimizations for Windows 10/11 Intel/AMD, rank evidence and uncertainty, and give exact before/after measurement steps and rollback. Do not recommend disabling Windows Defender, memory integrity, driver signing, security services or game anti-cheat. No automatic Windows edits. Distinguish FPS frame timing from input-to-photon, audio round-trip, DPC/ISR and network latency. Keep directions compact but technically precise. Never claim measured values not present."
	messages := make([]map[string]string, 0, len(x.History)+1)
	for _, t := range x.History {
		messages = append(messages, map[string]string{"role": t.Role, "content": t.Text})
	}
	question := x.Question
	if x.IncludeReport {
		question += "\n\nUser explicitly consented to send this sanitized Windows diagnostic JSON:\n" + reportForAI(report, true)
	}
	messages = append(messages, map[string]string{"role": "user", "content": question})
	payload := map[string]any{"model": modelID, "instructions": instructions, "input": messages, "store": false, "max_output_tokens": 1600}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/responses", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+x.ApiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("AI request failed: %w", err)
	}
	defer response.Body.Close()
	b, err := io.ReadAll(io.LimitReader(response.Body, 500000))
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI API returned HTTP %d. Check API key, model availability and billing status; no raw credentials are logged", response.StatusCode)
	}
	answer, err := parseResponse(b)
	if err != nil {
		return "", err
	}
	return answer, nil
}
func parseResponse(b []byte) (string, error) {
	var resp struct {
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return "", fmt.Errorf("Invalid AI response: %w", err)
	}
	var parts []string
	for _, o := range resp.Output {
		for _, c := range o.Content {
			if c.Type == "output_text" && c.Text != "" {
				parts = append(parts, c.Text)
			}
		}
	}
	if len(parts) == 0 {
		return "", errors.New("AI request returned no readable message; try again")
	}
	return strings.Join(parts, "\n"), nil
}
