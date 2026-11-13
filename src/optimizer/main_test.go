package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVDFPairs(t *testing.T) {
	vdf := `"AppState" { "appid" "2357570" "name" "SAWCHA Test" "installdir" "Sawcha Test Game" }`
	p := vdfPairs(vdf)
	for key, want := range map[string]string{"appid": "2357570", "name": "SAWCHA Test", "installdir": "Sawcha Test Game"} {
		if p[key] != want {
			t.Fatalf("%s got %q want %q", key, p[key], want)
		}
	}
}
func TestSteamLibParser(t *testing.T) {
	dir := t.TempDir()
	apps := filepath.Join(dir, "steamapps")
	if err := os.MkdirAll(apps, 0755); err != nil {
		t.Fatal(err)
	}
	data := `"libraryfolders" { "0" { "path" "` + dir + `" } }`
	if err := os.WriteFile(filepath.Join(apps, "libraryfolders.vdf"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	libs := readSteamLibs(dir)
	if len(libs) != 1 || libs[0] != dir {
		t.Fatalf("unexpected Steam paths %#v", libs)
	}
}
func TestFrameTimes(t *testing.T) {
	b := bytes.NewBufferString("Application,MsBetweenPresents,Other\n")
	for i := 0; i < 100; i++ {
		frame := 16.6667
		if i == 99 {
			frame = 50
		}
		fmt.Fprintf(b, "RainbowSix.exe,%.5f,0\n", frame)
	}
	bm, err := parseBenchmark(b)
	if err != nil {
		t.Fatal(err)
	}
	if bm.Frames != 100 || bm.AvgFPS < 56 || bm.AvgFPS > 61 || bm.Low1 >= bm.AvgFPS || bm.P99ms <= 16 {
		t.Fatalf("unexpected metrics %+v", bm)
	}
}
func TestFrameTimeMissing(t *testing.T) {
	_, e := parseBenchmark(strings.NewReader("Nope,Neither\n1,2\n"))
	if e == nil || !strings.Contains(e.Error(), "frame-time") {
		t.Fatalf("want helpful validation, got %v", e)
	}
}
func TestAuthAndFrontend(t *testing.T) {
	a := &App{token: "abc", root: t.TempDir(), hardware: map[string]interface{}{}, games: []Game{}, advice: []Advice{}, config: SavedConfig{Preset: "Stock"}, live: map[string]interface{}{}}
	a.initAuth()
	srv := httptest.NewServer(a.routes())
	defer srv.Close()
	resp, e := http.Get(srv.URL + "/")
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal(resp.Status)
	}
	req, _ := http.NewRequest("GET", srv.URL+"/api/state", nil)
	resp, e = http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("unauthenticated request got %d", resp.StatusCode)
	}
	req, _ = http.NewRequest("GET", srv.URL+"/api/state", nil)
	req.Header.Set("X-Sawcha-Token", "abc")
	resp, e = http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	var got map[string]interface{}
	if e := json.NewDecoder(resp.Body).Decode(&got); e != nil {
		t.Fatal(e)
	}
	if got["error"] != "Sign in to use the optimizer" {
		t.Fatalf("expected sign-in required; got %#v", got)
	}
	cookieJar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: cookieJar}
	login, _ := http.NewRequest("POST", srv.URL+"/api/auth/guest", strings.NewReader("{}"))
	login.Header.Set("X-Sawcha-Token", "abc")
	resp, e = client.Do(login)
	if e != nil || resp.StatusCode != 200 {
		t.Fatalf("guest login failed: %v, %v", e, resp.Status)
	}
	resp.Body.Close()
	req, _ = http.NewRequest("GET", srv.URL+"/api/state", nil)
	req.Header.Set("X-Sawcha-Token", "abc")
	resp, e = client.Do(req)
	if e != nil || resp.StatusCode != 200 {
		t.Fatalf("authenticated state failed: %v %v", e, resp.Status)
	}
	defer resp.Body.Close()
	if e = json.NewDecoder(resp.Body).Decode(&got); e != nil {
		t.Fatal(e)
	}
	if got["version"] != version {
		t.Fatalf("wrong version: %v", got["version"])
	}
}
func TestDedupe(t *testing.T) {
	games := dedupeGames([]Game{{Name: "Gamma", Platform: "Custom", InstallDir: "C:/A"}, {Name: "Gamma", Platform: "Custom", InstallDir: "C:/A"}, {Name: "Alpha", Platform: "Steam", InstallDir: "C:/B"}})
	if len(games) != 2 || games[0].Name != "Alpha" {
		t.Fatalf("bad dedupe %+v", games)
	}
}
