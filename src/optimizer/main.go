package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const version = "8.0.0-native"

//go:embed web/*
var assets embed.FS

type Game struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	InstallDir string `json:"installDir"`
	Exe        string `json:"exe"`
	URI        string `json:"uri,omitempty"`
	Source     string `json:"source"`
	Custom     bool   `json:"custom"`
}
type Advice struct {
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Action   string `json:"action,omitempty"`
}
type SavedConfig struct {
	ManualGames         []Game                        `json:"manualGames"`
	ExeOverrides        map[string]string             `json:"exeOverrides"`
	PresentMonPath      string                        `json:"presentMonPath"`
	Backup              *Backup                       `json:"backup,omitempty"`
	Preset              string                        `json:"preset"`
	SensitivityProfiles map[string]SensitivitySetting `json:"sensitivityProfiles,omitempty"`
}
type RegBackup struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Exists bool   `json:"exists"`
	Kind   string `json:"kind"`
	Data   string `json:"data"`
}
type Backup struct {
	Entries   []RegBackup `json:"entries"`
	PowerGUID string      `json:"powerGUID"`
	Created   string      `json:"created"`
}
type Job struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Status  string `json:"status"`
	Started string `json:"started"`
	Ended   string `json:"ended,omitempty"`
	Output  string `json:"output"`
}
type Benchmark struct {
	ID       string    `json:"id"`
	Game     string    `json:"game"`
	Label    string    `json:"label"`
	Profile  string    `json:"profile"`
	Date     string    `json:"date"`
	Frames   int       `json:"frames"`
	AvgFPS   float64   `json:"avgFPS"`
	Low1     float64   `json:"low1"`
	Low01    float64   `json:"low01"`
	P99ms    float64   `json:"p99ms"`
	Medianms float64   `json:"medianms"`
	Stutters int       `json:"stutters"`
	Chart    []float64 `json:"chart"`
	Source   string    `json:"source"`
}

type App struct {
	auth        authEngine
	mu          sync.RWMutex
	token       string
	admin       bool
	lastMonitor time.Time
	preferences AppPreferences
	runningGame string
	root        string
	port        int
	stage       string
	progress    int
	ready       bool
	scanError   string
	scanned     string
	hardware    map[string]interface{}
	games       []Game
	advice      []Advice
	config      SavedConfig
	live        map[string]interface{}
	jobs        []Job
	history     []Benchmark
	capturing   bool
	logEntries  []string
}

func main() {
	root := appFolder()
	_ = os.MkdirAll(root, 0755)
	importLegacySettings(root)
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	a := &App{root: root, admin: isElevated(), token: hex.EncodeToString(b), stage: "Initializing engine", progress: 2, hardware: map[string]interface{}{}, games: []Game{}, advice: []Advice{}, jobs: []Job{}, history: []Benchmark{}, live: map[string]interface{}{}, config: SavedConfig{ExeOverrides: map[string]string{}, Preset: "Stock"}}
	a.loadConfig()
	a.loadAppPreferences()
	a.loadHistory()
	a.initAuth()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	a.port = listener.Addr().(*net.TCPAddr).Port
	server := &http.Server{Handler: a.routes(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Println(err)
		}
	}()
	go a.scan()
	go a.pollLoop()
	go a.gameWatchLoop()
	runNative(a)
	_ = server.Close()
}

func appFolder() string {
	p := os.Getenv("LOCALAPPDATA")
	if p == "" {
		d, _ := os.UserConfigDir()
		p = d
	}
	if p == "" {
		p = "."
	}
	return filepath.Join(p, "Sawcha", "Optimizer")
}

// importLegacySettings copies only user-specific non-secret preferences and benchmark
// history from the previous branded release. We never move or delete old files.
func importLegacySettings(root string) {
	legacy := filepath.Join(filepath.Dir(filepath.Dir(root)), "RageWeezy", "Optimizer")
	for _, filename := range []string{"settings.json", "benchmarks.json"} {
		newPath := filepath.Join(root, filename)
		if _, err := os.Stat(newPath); err == nil {
			continue
		}
		oldPath := filepath.Join(legacy, filename)
		data, err := os.ReadFile(oldPath)
		if err != nil || len(data) == 0 || len(data) > 10<<20 {
			continue
		}
		_ = os.WriteFile(newPath, data, 0600)
	}
}

func (a *App) logger(msg string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.logEntries = append([]string{time.Now().Format("15:04:05") + "  " + msg}, a.logEntries...)
	if len(a.logEntries) > 120 {
		a.logEntries = a.logEntries[:120]
	}
}
func (a *App) updateStage(stage string, progress int) {
	a.mu.Lock()
	a.stage = stage
	a.progress = progress
	a.mu.Unlock()
	a.logger(stage)
}
func (a *App) loadConfig() {
	b, err := os.ReadFile(filepath.Join(a.root, "settings.json"))
	if err == nil {
		_ = json.Unmarshal(b, &a.config)
	}
	if a.config.ExeOverrides == nil {
		a.config.ExeOverrides = map[string]string{}
	}
	if a.config.SensitivityProfiles == nil {
		a.config.SensitivityProfiles = map[string]SensitivitySetting{}
	}
	if a.config.Preset == "" {
		a.config.Preset = "Stock"
	}
}
func (a *App) persistConfig() error {
	a.mu.RLock()
	cfg := a.config
	a.mu.RUnlock()
	return writeJSON(filepath.Join(a.root, "settings.json"), cfg)
}
func writeJSON(name string, v interface{}) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	temp := name + ".tmp"
	if e = os.WriteFile(temp, b, 0600); e != nil {
		return e
	}
	return os.Rename(temp, name)
}
func (a *App) loadHistory() {
	b, e := os.ReadFile(filepath.Join(a.root, "benchmarks.json"))
	if e == nil {
		_ = json.Unmarshal(b, &a.history)
	}
	if a.history == nil {
		a.history = []Benchmark{}
	}
}
func (a *App) persistHistory() error {
	a.mu.RLock()
	h := append([]Benchmark(nil), a.history...)
	a.mu.RUnlock()
	return writeJSON(filepath.Join(a.root, "benchmarks.json"), h)
}
func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			b, e := assets.ReadFile("web/index.html")
			if e != nil {
				http.Error(w, e.Error(), 500)
				return
			}
			s := strings.ReplaceAll(string(b), "__SAWCHA_TOKEN__", a.token)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = io.WriteString(w, s)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/web/") {
			sub, e := fs.Sub(assets, "web")
			if e == nil {
				http.StripPrefix("/web/", http.FileServer(http.FS(sub))).ServeHTTP(w, r)
				return
			}
		}
		http.NotFound(w, r)
	})
	// Prevent a foreign page from treating a local HTTP service as its own endpoint.
	mux.HandleFunc("/auth/google/callback", a.googleCallback)
	api := func(path string, handler func(http.ResponseWriter, *http.Request)) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Sawcha-Token") != a.token {
				http.Error(w, "Unauthorized", 403)
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" && origin != fmt.Sprintf("http://127.0.0.1:%d", a.port) {
				bad(w, "Foreign origin blocked", 403)
				return
			}
			if !strings.HasPrefix(path, "/api/auth/") {
				if _, ok := a.loginIdentity(r); !ok {
					bad(w, "Sign in to use the optimizer", 401)
					return
				}
			}
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			handler(w, r)
		})
	}
	api("/api/auth/status", a.authStatus)
	api("/api/auth/guest", a.authGuest)
	api("/api/auth/logout", a.authLogout)
	api("/api/auth/google/configure", a.googleConfigure)
	api("/api/auth/google/start", a.googleStart)
	api("/api/auth/google/status", a.googleStatus)
	api("/api/auth/google/finish", a.googleFinish)
	api("/api/auth/developer/setup", a.developerSetup)
	api("/api/auth/developer/unlock", a.developerUnlock)
	api("/api/auth/developer/lock", a.developerLock)
	api("/api/developer/report", a.developerReport)
	api("/api/developer/status", func(w http.ResponseWriter, r *http.Request) {
		id, ok := a.loginIdentity(r)
		if !ok || id.Role != "developer" {
			bad(w, "Developer role required", 403)
			return
		}
		send(w, developerVerifyData(a))
	})
	api("/api/state", a.stateHandler)
	api("/api/live", a.liveHandler)
	api("/api/scan", a.scanHandler)
	api("/api/games/add", a.addGameHandler)
	api("/api/games/exe", a.setGameExeHandler)
	api("/api/games/launch", a.launchGameHandler)
	api("/api/games/remove", a.removeGameHandler)
	api("/api/preset", a.presetHandler)
	api("/api/restore", a.restoreHandler)
	api("/api/tool", a.toolHandler)
	api("/api/diagnostic", a.diagnosticHandler)
	api("/api/ping", a.pingHandler)
	api("/api/mouse", a.mouseHandler)
	api("/api/benchmark", a.benchmarkHandler)
	api("/api/benchmark/import", a.importHandler)
	api("/api/benchmark/capture", a.captureHandler)
	api("/api/settings", a.settingsHandler)
	api("/api/preferences", a.preferencesHandler)
	api("/api/guides", a.guidesHandler)
	api("/api/sensitivity", a.sensitivityHandler)
	api("/api/assistant/chat", a.advisorHandler)
	api("/api/quit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		send(w, map[string]bool{"ok": true})
		go func() { time.Sleep(500 * time.Millisecond); os.Exit(0) }()
	})
	return mux
}
func send(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(data)
}
func bad(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	send(w, map[string]string{"error": msg})
}
func decode(r *http.Request, v interface{}) error {
	if r.Method != "POST" {
		return errors.New("POST required")
	}
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}
func (a *App) stateHandler(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	send(w, map[string]interface{}{"version": version, "stage": a.stage, "progress": a.progress, "ready": a.ready, "scanError": a.scanError, "scanned": a.scanned, "hardware": a.hardware, "games": a.games, "advice": a.advice, "config": a.config, "live": a.live, "jobs": a.jobs, "history": a.history, "logs": a.logEntries, "capturing": a.capturing, "folder": a.root, "administrator": a.admin, "presentMonFound": findPresentMon(a.config.PresentMonPath) != "", "gameGuard": map[string]any{"running": a.runningGame, "preferences": a.preferences}, "identity": func() LoginIdentity { id, _ := a.loginIdentity(r); return id }()})
}
func (a *App) liveHandler(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	a.lastMonitor = time.Now()
	data := a.live
	a.mu.Unlock()
	send(w, data)
}
func (a *App) scanHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		bad(w, "POST required", 405)
		return
	}
	var in struct {
		AckGameRunning bool `json:"ack_game_running"`
	}
	if e := decode(r, &in); e != nil {
		bad(w, e.Error(), 400)
		return
	}
	if e := a.changeDuringGameWarning(in.AckGameRunning); e != nil {
		bad(w, e.Error(), 409)
		return
	}
	a.mu.RLock()
	busy := !a.ready
	a.mu.RUnlock()
	if busy {
		bad(w, "Scan is running", 409)
		return
	}
	go a.scan()
	send(w, map[string]bool{"ok": true})
}
func run(ctx context.Context, exe string, args ...string) (string, error) {
	c := exec.CommandContext(ctx, exe, args...)
	prepareHidden(c)
	c.Env = os.Environ()
	b, e := c.CombinedOutput()
	if len(b) > 120000 {
		b = b[:120000]
	}
	return strings.TrimSpace(string(b)), e
}
func powershell(ctx context.Context, script string) (string, error) {
	return run(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; $ProgressPreference='SilentlyContinue'; "+script)
}
func powerJSON(script string, timeout time.Duration) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := powershell(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("Windows query failed: %v: %s", err, short(out, 300))
	}
	idx := strings.Index(out, "{")
	if idx < 0 {
		return nil, errors.New("Windows returned no JSON data: " + short(out, 220))
	}
	var result map[string]interface{}
	if err = json.Unmarshal([]byte(out[idx:]), &result); err != nil {
		return nil, fmt.Errorf("Invalid Windows data: %v", err)
	}
	return result, nil
}
func short(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

const systemPS = `$ErrorActionPreference='SilentlyContinue';
$c=Get-CimInstance Win32_Processor | Select-Object Name,NumberOfCores,NumberOfLogicalProcessors,MaxClockSpeed,LoadPercentage;
$g=@(Get-CimInstance Win32_VideoController | Select-Object Name,DriverVersion,AdapterRAM,CurrentHorizontalResolution,CurrentVerticalResolution,CurrentRefreshRate,VideoProcessor);
$o=Get-CimInstance Win32_OperatingSystem | Select-Object Caption,Version,BuildNumber,LastBootUpTime,TotalVisibleMemorySize,FreePhysicalMemory;
$m=@(Get-CimInstance Win32_PhysicalMemory | Select-Object Capacity,Speed,ConfiguredClockSpeed,Manufacturer,PartNumber);
$d=@(Get-CimInstance Win32_LogicalDisk -Filter 'DriveType=3' | Select-Object DeviceID,VolumeName,Size,FreeSpace,FileSystem);
$b=Get-CimInstance Win32_BaseBoard | Select-Object Manufacturer,Product;
$n=@(Get-CimInstance Win32_NetworkAdapter -Filter 'NetEnabled=True' | Select-Object Name,Speed,MACAddress,NetConnectionID);
$x=@(Get-CimInstance Win32_PnPEntity | Where-Object { $_.Name -match 'Xbox.*Controller|Gamepad|DualSense|DualShock|Wireless Controller' } | Select-Object -First 12 Name,Status);
$p=@(Get-CimInstance Win32_PointingDevice | Select-Object -First 12 Name,DeviceID);
$sys=[ordered]@{ CPU=$c; GPU=$g; OS=$o; RAM=$m; Disks=$d; Board=$b; Network=$n; Controllers=$x; Mice=$p; Computer=$env:COMPUTERNAME };
$sys | ConvertTo-Json -Depth 5 -Compress`

func (a *App) scan() {
	a.mu.Lock()
	a.ready = false
	a.progress = 3
	a.stage = "Connecting to Windows diagnostics"
	a.scanError = ""
	a.mu.Unlock()
	a.updateStage("Reading CPU, BIOS and memory", 12)
	snap, err := powerJSON(systemPS, 36*time.Second)
	if err != nil {
		snap = map[string]interface{}{"OS": map[string]interface{}{"Caption": "Windows 10 / 11", "Version": "Unavailable"}, "scanWarning": err.Error()}
		a.logger("Hardware scan issue: " + err.Error())
	}
	a.updateStage("Inspecting NVIDIA GPU and graphics drivers", 37)
	snap["nvidia"] = readNvidia()
	snap["mouseSpeed"] = readMouseSpeed()
	snap["powerPlan"] = currentPowerPlan()
	snap["platform"] = runtime.GOOS
	a.updateStage("Locating Steam libraries and manifests", 53)
	games := scanSteam()
	a.updateStage("Checking Epic Games, Riot and other launchers", 72)
	games = append(games, scanEpic()...)
	games = append(games, scanLocalLaunchers()...)
	a.mu.RLock()
	cfg := a.config
	a.mu.RUnlock()
	games = append(games, cfg.ManualGames...)
	games = dedupeGames(games)
	for i := range games {
		if ex := cfg.ExeOverrides[games[i].ID]; ex != "" {
			games[i].Exe = ex
		}
	}
	a.updateStage("Generating device-specific recommendations", 88)
	adv := recommend(snap, games)
	a.mu.Lock()
	a.hardware = snap
	a.games = games
	a.advice = adv
	a.scanned = time.Now().Format("Jan 02, 2006 3:04 PM")
	a.ready = true
	a.progress = 100
	a.stage = "All systems ready"
	if err != nil {
		a.scanError = err.Error()
	}
	a.mu.Unlock()
	a.logger(fmt.Sprintf("Scan complete. %d game(s) detected.", len(games)))
}
func readNvidia() interface{} {
	candidates := []string{"nvidia-smi.exe"}
	for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if base != "" {
			candidates = append(candidates, filepath.Join(base, "NVIDIA Corporation", "NVSMI", "nvidia-smi.exe"))
		}
	}
	for _, exe := range candidates {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		out, err := run(ctx, exe, "--query-gpu=name,driver_version,temperature.gpu,utilization.gpu,memory.used,memory.total,power.draw", "--format=csv,noheader,nounits")
		cancel()
		if err == nil && out != "" {
			lines := strings.Split(out, "\n")
			g := []map[string]interface{}{}
			for _, line := range lines {
				parts := strings.Split(line, ",")
				if len(parts) >= 7 {
					for i := range parts {
						parts[i] = strings.TrimSpace(parts[i])
					}
					g = append(g, map[string]interface{}{"name": parts[0], "driver": parts[1], "tempC": toNumber(parts[2]), "usage": toNumber(parts[3]), "vramUsedMB": toNumber(parts[4]), "vramTotalMB": toNumber(parts[5]), "powerW": toNumber(parts[6])})
				}
			}
			if len(g) > 0 {
				return g
			}
		}
	}
	return []map[string]interface{}{}
}
func toNumber(s string) float64 { v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64); return v }
func currentPowerPlan() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, e := run(ctx, "powercfg.exe", "/getactivescheme")
	if e != nil {
		return "Unavailable"
	}
	return short(out, 240)
}
func readMouseSpeed() int { return systemMouseSpeed() }
func isElevated() bool    { return adminCheck() }

const livePS = `$ErrorActionPreference='SilentlyContinue';
$c=Get-CimInstance Win32_Processor | Select-Object -First 1 Name,LoadPercentage,CurrentClockSpeed;
$o=Get-CimInstance Win32_OperatingSystem | Select-Object TotalVisibleMemorySize,FreePhysicalMemory;
$top=@(Get-Process | Sort-Object WorkingSet64 -Descending | Select-Object -First 7 ProcessName,Id,WorkingSet64,CPU);
@{CPU=$c;OS=$o;Processes=$top;Timestamp=(Get-Date).ToString('HH:mm:ss')} | ConvertTo-Json -Depth 4 -Compress`

func (a *App) pollLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	lastSample := time.Time{}
	for range ticker.C {
		a.mu.RLock()
		recentlyViewed := time.Since(a.lastMonitor) < 18*time.Second
		a.mu.RUnlock()
		interval := 30 * time.Second
		if recentlyViewed {
			interval = 5 * time.Second
		}
		if a.isQuietGaming() {
			_, p := a.guardSnapshot()
			interval = time.Duration(p.QuietMonitorSeconds) * time.Second
		}
		if time.Since(lastSample) < interval {
			continue
		}
		lastSample = time.Now()
		data, err := powerJSON(livePS, 11*time.Second)
		if err == nil {
			data["nvidia"] = readNvidia()
			a.mu.Lock()
			a.live = data
			a.mu.Unlock()
		}
	}
}

func recommend(h map[string]interface{}, games []Game) []Advice {
	a := []Advice{}
	cpu, _ := h["CPU"].(map[string]interface{})
	osData, _ := h["OS"].(map[string]interface{})
	name, _ := cpu["Name"].(string)
	mem := 0.0
	if osData != nil {
		mem = toFloat(osData["TotalVisibleMemorySize"]) / 1048576
	}
	if mem > 0 && mem < 12 {
		a = append(a, Advice{"warn", "Limited system memory", fmt.Sprintf("%.1f GB RAM detected. For modern games, close browsers and background apps first; 16 GB or more often helps.", mem), "processes"})
	}
	if strings.Contains(strings.ToLower(name), "ryzen") {
		a = append(a, Advice{"info", "AMD Ryzen platform", "Check your motherboard BIOS and AMD chipset driver versions using the manufacturer's official tools.", "drivers"})
	}
	nvidia, _ := h["nvidia"].([]map[string]interface{})
	if len(nvidia) > 0 {
		v := toFloat(nvidia[0]["vramTotalMB"])
		if v > 0 && v <= 6144 {
			a = append(a, Advice{"warn", "VRAM budget matters", fmt.Sprintf("%.0f MB GPU memory reported. Lower texture quality before dropping resolution or using risky registry tweaks.", v), "video"})
		}
		a = append(a, Advice{"good", "NVIDIA telemetry online", "GPU temperature, driver, utilization and video memory can be sampled in the Live Monitor.", "monitor"})
	} else {
		a = append(a, Advice{"info", "GPU sensor support", "Install the official NVIDIA driver and nvidia-smi for direct NVIDIA GPU telemetry; otherwise Windows WMI still lists your adapter.", "drivers"})
	}
	if len(games) == 0 {
		a = append(a, Advice{"info", "Game library not found", "Steam and Epic are scanned automatically. You can also add any game's EXE manually.", "games"})
	} else {
		a = append(a, Advice{"good", "Games indexed", fmt.Sprintf("%d installed game(s) identified. Choose a game to open it or capture a performance benchmark.", len(games)), "games"})
	}
	for _, v := range asMaps(h["Disks"]) {
		size := toFloat(v["Size"])
		free := toFloat(v["FreeSpace"])
		if size > 0 && free/size < .12 {
			a = append(a, Advice{"warn", "Low drive free space", fmt.Sprintf("Drive %v has %.0f%% free. Keep space available for updates, shader cache and recording.", v["DeviceID"], 100*free/size), "hardware"})
		}
	}
	return a
}
func toFloat(x interface{}) float64 {
	switch v := x.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	case string:
		return toNumber(v)
	}
	return 0
}
func asMaps(raw interface{}) []map[string]interface{} {
	o := []map[string]interface{}{}
	switch v := raw.(type) {
	case []interface{}:
		for _, x := range v {
			if m, ok := x.(map[string]interface{}); ok {
				o = append(o, m)
			}
		}
	case []map[string]interface{}:
		return v
	case map[string]interface{}:
		return []map[string]interface{}{v}
	}
	return o
}

func idFor(platform, path string) string {
	h := sha256.Sum256([]byte(strings.ToLower(platform + "|" + path)))
	return hex.EncodeToString(h[:6])
}
func dedupeGames(g []Game) []Game {
	seen := map[string]bool{}
	out := []Game{}
	for _, game := range g {
		if game.ID == "" {
			game.ID = idFor(game.Platform, game.InstallDir+game.Name)
		}
		if seen[game.ID] || strings.TrimSpace(game.Name) == "" {
			continue
		}
		seen[game.ID] = true
		out = append(out, game)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}
func fileExists(p string) bool  { f, e := os.Stat(p); return e == nil && !f.IsDir() }
func dirExists(p string) bool   { f, e := os.Stat(p); return e == nil && f.IsDir() }
func drivePath(p string) string { p = strings.ReplaceAll(p, `\\`, `\`); return p }
func regValue(key, value string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	out, e := run(ctx, "reg.exe", "query", key, "/v", value)
	if e != nil {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(strings.ToLower(line), strings.ToLower(value)) && strings.Contains(line, "REG_") {
			parts := regexp.MustCompile(`\s{2,}`).Split(strings.TrimSpace(line), 3)
			if len(parts) == 3 {
				return strings.TrimSpace(parts[2])
			}
		}
	}
	return ""
}
func steamRoots() []string {
	paths := []string{}
	for _, key := range []string{`HKCU\Software\Valve\Steam`, `HKLM\SOFTWARE\WOW6432Node\Valve\Steam`} {
		for _, v := range []string{"SteamPath", "InstallPath"} {
			p := regValue(key, v)
			if p != "" {
				paths = append(paths, filepath.Clean(drivePath(p)))
			}
		}
	}
	for _, base := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles")} {
		if base != "" {
			paths = append(paths, filepath.Join(base, "Steam"))
		}
	}
	return uniqueDirs(paths)
}
func uniqueDirs(paths []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, p := range paths {
		p = filepath.Clean(p)
		k := strings.ToLower(p)
		if !seen[k] && dirExists(p) {
			out = append(out, p)
			seen[k] = true
		}
	}
	return out
}

var vdfProp = regexp.MustCompile(`(?m)"([A-Za-z0-9_]+)"\s+"((?:\\.|[^"\\])*)"`)

func vdfPairs(s string) map[string]string {
	out := map[string]string{}
	for _, m := range vdfProp.FindAllStringSubmatch(s, -1) {
		out[m[1]] = strings.ReplaceAll(m[2], `\\`, `\`)
	}
	return out
}
func readSteamLibs(root string) []string {
	libs := []string{root}
	f, err := os.ReadFile(filepath.Join(root, "steamapps", "libraryfolders.vdf"))
	if err != nil {
		return uniqueDirs(libs)
	}
	for _, m := range vdfProp.FindAllStringSubmatch(string(f), -1) {
		if strings.EqualFold(m[1], "path") || regexp.MustCompile(`^\d+$`).MatchString(m[1]) {
			p := drivePath(m[2])
			if dirExists(p) {
				libs = append(libs, p)
			}
		}
	}
	return uniqueDirs(libs)
}
func scanSteam() []Game {
	out := []Game{}
	seen := map[string]bool{}
	for _, root := range steamRoots() {
		for _, lib := range readSteamLibs(root) {
			apps := filepath.Join(lib, "steamapps")
			files, err := filepath.Glob(filepath.Join(apps, "appmanifest_*.acf"))
			if err != nil {
				continue
			}
			for _, f := range files {
				b, e := os.ReadFile(f)
				if e != nil {
					continue
				}
				props := vdfPairs(string(b))
				id, name, folder := props["appid"], props["name"], props["installdir"]
				if id == "" || name == "" {
					continue
				}
				if seen[id] {
					continue
				}
				seen[id] = true
				dir := filepath.Join(apps, "common", folder)
				out = append(out, Game{ID: "steam-" + id, Name: name, Platform: "Steam", InstallDir: dir, Exe: findLikelyExe(dir), URI: "steam://rungameid/" + id, Source: "Steam appmanifest"})
			}
		}
	}
	return out
}
func scanEpic() []Game {
	base := os.Getenv("PROGRAMDATA")
	if base == "" {
		return nil
	}
	files, _ := filepath.Glob(filepath.Join(base, "Epic", "EpicGamesLauncher", "Data", "Manifests", "*.item"))
	out := []Game{}
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var item struct {
			DisplayName      string
			InstallLocation  string
			AppName          string
			LaunchExecutable string
			CatalogNamespace string
		}
		if json.Unmarshal(b, &item) != nil || item.DisplayName == "" || item.InstallLocation == "" {
			continue
		}
		exe := filepath.Join(item.InstallLocation, item.LaunchExecutable)
		if !fileExists(exe) {
			exe = findLikelyExe(item.InstallLocation)
		}
		uri := ""
		if item.AppName != "" {
			uri = "com.epicgames.launcher://apps/" + url.PathEscape(item.AppName) + "?action=launch&silent=true"
		}
		out = append(out, Game{ID: idFor("epic", item.AppName+item.InstallLocation), Name: item.DisplayName, Platform: "Epic Games", InstallDir: item.InstallLocation, Exe: exe, URI: uri, Source: "Epic manifest"})
	}
	return out
}
func scanLocalLaunchers() []Game {
	roots := []struct{ path, platform string }{}
	for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if base != "" {
			roots = append(roots, struct{ path, platform string }{filepath.Join(base, "EA Games"), "EA App"})
			roots = append(roots, struct{ path, platform string }{filepath.Join(base, "GOG Galaxy", "Games"), "GOG"})
			roots = append(roots, struct{ path, platform string }{filepath.Join(base, "Ubisoft", "Ubisoft Game Launcher", "games"), "Ubisoft"})
		}
	}
	if drive := os.Getenv("SystemDrive"); drive != "" {
		roots = append(roots, struct{ path, platform string }{filepath.Join(drive+`\`, "Riot Games"), "Riot"})
	}
	out := []Game{}
	for _, root := range roots {
		if !dirExists(root.path) {
			continue
		}
		entries, e := os.ReadDir(root.path)
		if e != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if strings.EqualFold(name, "Riot Client") || strings.EqualFold(name, "Redistributables") {
				continue
			}
			p := filepath.Join(root.path, name)
			exe := findLikelyExe(p)
			if exe == "" {
				continue
			}
			out = append(out, Game{ID: idFor(root.platform, p), Name: name, Platform: root.platform, InstallDir: p, Exe: exe, Source: "Installed game folder"})
		}
	}
	return out
}
func findLikelyExe(dir string) string {
	if !dirExists(dir) {
		return ""
	}
	type folder struct {
		path  string
		depth int
	}
	q := []folder{{dir, 0}}
	best := ""
	bestScore := -999
	visited := 0
	for len(q) > 0 && visited < 320 {
		current := q[0]
		q = q[1:]
		entries, err := os.ReadDir(current.path)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			visited++
			if visited > 320 {
				break
			}
			name := entry.Name()
			p := filepath.Join(current.path, name)
			if entry.IsDir() {
				if current.depth < 2 && !strings.HasPrefix(name, ".") && len(q) < 140 {
					q = append(q, folder{p, current.depth + 1})
				}
				continue
			}
			if !strings.EqualFold(filepath.Ext(name), ".exe") {
				continue
			}
			low := strings.ToLower(name)
			if strings.Contains(low, "unins") || strings.Contains(low, "crash") || strings.Contains(low, "setup") || strings.Contains(low, "unitycrash") || strings.Contains(low, "easyanticheat") || strings.Contains(low, "updat") || strings.Contains(low, "redist") || strings.Contains(low, "prereq") {
				continue
			}
			score := 10 - current.depth*3
			if strings.Contains(strings.ToLower(p), "win64") {
				score += 9
			}
			if strings.Contains(low, "shipping") {
				score += 5
			}
			if strings.Contains(low, "launcher") {
				score -= 4
			}
			if score > bestScore {
				bestScore = score
				best = p
			}
		}
	}
	return best
}

type reqGame struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Exe      string `json:"exe"`
	Platform string `json:"platform"`
}

func (a *App) addGameHandler(w http.ResponseWriter, r *http.Request) {
	var v reqGame
	if e := decode(r, &v); e != nil {
		bad(w, e.Error(), 400)
		return
	}
	v.Name = strings.TrimSpace(v.Name)
	v.Exe = filepath.Clean(strings.TrimSpace(strings.Trim(v.Exe, `"`)))
	if v.Name == "" || !fileExists(v.Exe) || !strings.EqualFold(filepath.Ext(v.Exe), ".exe") {
		bad(w, "Enter a name and an existing Windows .exe path.", 400)
		return
	}
	g := Game{ID: idFor("manual", v.Exe), Name: v.Name, Platform: "Custom", Exe: v.Exe, InstallDir: filepath.Dir(v.Exe), Source: "Added manually", Custom: true}
	a.mu.Lock()
	a.config.ManualGames = append(a.config.ManualGames, g)
	a.games = dedupeGames(append(a.games, g))
	a.mu.Unlock()
	_ = a.persistConfig()
	a.logger("Game added: " + g.Name)
	send(w, map[string]interface{}{"ok": true, "game": g})
}
func (a *App) setGameExeHandler(w http.ResponseWriter, r *http.Request) {
	var v reqGame
	if e := decode(r, &v); e != nil {
		bad(w, e.Error(), 400)
		return
	}
	v.Exe = filepath.Clean(strings.TrimSpace(strings.Trim(v.Exe, `"`)))
	if !fileExists(v.Exe) || !strings.EqualFold(filepath.Ext(v.Exe), ".exe") {
		bad(w, "EXE not found. Enter the exact path to the game process.", 400)
		return
	}
	a.mu.Lock()
	found := false
	for i := range a.games {
		if a.games[i].ID == v.ID {
			a.games[i].Exe = v.Exe
			found = true
		}
	}
	if found {
		a.config.ExeOverrides[v.ID] = v.Exe
	}
	a.mu.Unlock()
	if !found {
		bad(w, "Game not found", 404)
		return
	}
	_ = a.persistConfig()
	send(w, map[string]bool{"ok": true})
}
func (a *App) removeGameHandler(w http.ResponseWriter, r *http.Request) {
	var v reqGame
	if e := decode(r, &v); e != nil {
		bad(w, e.Error(), 400)
		return
	}
	a.mu.Lock()
	removed := false
	for i, g := range a.config.ManualGames {
		if g.ID == v.ID {
			a.config.ManualGames = append(a.config.ManualGames[:i], a.config.ManualGames[i+1:]...)
			removed = true
			break
		}
	}
	if removed {
		for i, g := range a.games {
			if g.ID == v.ID {
				a.games = append(a.games[:i], a.games[i+1:]...)
				break
			}
		}
	}
	a.mu.Unlock()
	if !removed {
		bad(w, "Only manual games can be removed", 400)
		return
	}
	_ = a.persistConfig()
	send(w, map[string]bool{"ok": true})
}
func (a *App) lookupGame(id string) (Game, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, g := range a.games {
		if g.ID == id {
			return g, true
		}
	}
	return Game{}, false
}
func (a *App) launchGameHandler(w http.ResponseWriter, r *http.Request) {
	var v reqGame
	if e := decode(r, &v); e != nil {
		bad(w, e.Error(), 400)
		return
	}
	g, ok := a.lookupGame(v.ID)
	if !ok {
		bad(w, "Unknown game", 404)
		return
	}
	var e error
	if strings.HasPrefix(g.URI, "steam://rungameid/") || strings.HasPrefix(g.URI, "com.epicgames.launcher://apps/") {
		e = exec.Command("explorer.exe", g.URI).Start()
	} else if fileExists(g.Exe) {
		e = exec.Command(g.Exe).Start()
	} else {
		bad(w, "No playable executable found. Use Set EXE to correct the path.", 400)
		return
	}
	if e != nil {
		bad(w, "Could not launch: "+e.Error(), 500)
		return
	}
	a.logger("Launch requested: " + g.Name)
	send(w, map[string]bool{"ok": true})
}

type reqPreset struct {
	AckGameRunning bool   `json:"ack_game_running"`
	Profile        string `json:"profile"`
	Power          bool   `json:"power"`
}
type RegChoice struct {
	Key   string
	Name  string
	Value string
}

var regChoices = []RegChoice{
	{`HKCU\Software\Microsoft\GameBar`, "AutoGameModeEnabled", "1"},
	{`HKCU\Software\Microsoft\GameBar`, "AllowAutoGameMode", "1"},
	{`HKCU\System\GameConfigStore`, "GameDVR_Enabled", "0"},
	{`HKCU\SOFTWARE\Microsoft\Windows\CurrentVersion\GameDVR`, "AppCaptureEnabled", "0"},
}
var regLine = regexp.MustCompile(`(?i)^\s*([^\s]+)\s+(REG_\w+)\s+(.*?)\s*$`)
var guidPattern = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

func snapshotReg(choice RegChoice) RegBackup {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	out, err := run(ctx, "reg.exe", "query", choice.Key, "/v", choice.Name)
	b := RegBackup{Key: choice.Key, Name: choice.Name}
	if err != nil {
		return b
	}
	for _, line := range strings.Split(out, "\n") {
		m := regLine.FindStringSubmatch(line)
		if len(m) == 4 && strings.EqualFold(m[1], choice.Name) {
			b.Exists = true
			b.Kind = m[2]
			b.Data = m[3]
			break
		}
	}
	return b
}
func makeBackup() *Backup {
	backup := &Backup{Created: time.Now().Format(time.RFC3339)}
	for _, v := range regChoices {
		backup.Entries = append(backup.Entries, snapshotReg(v))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, _ := run(ctx, "powercfg.exe", "/getactivescheme")
	backup.PowerGUID = guidPattern.FindString(out)
	return backup
}
func regSet(choice RegChoice) error {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, e := run(ctx, "reg.exe", "add", choice.Key, "/v", choice.Name, "/t", "REG_DWORD", "/d", choice.Value, "/f")
	if e != nil {
		return fmt.Errorf("Registry update %s: %v %s", choice.Name, e, short(out, 160))
	}
	return nil
}
func (a *App) presetHandler(w http.ResponseWriter, r *http.Request) {
	var v reqPreset
	if e := decode(r, &v); e != nil {
		bad(w, e.Error(), 400)
		return
	}
	if e := a.changeDuringGameWarning(v.AckGameRunning); e != nil {
		bad(w, e.Error(), 409)
		return
	}
	if v.Profile != "Competitive" && v.Profile != "Balanced" && v.Profile != "Creator" {
		bad(w, "Unknown preset", 400)
		return
	}
	a.mu.RLock()
	bk := a.config.Backup
	a.mu.RUnlock()
	if bk == nil {
		bk = makeBackup()
		a.mu.Lock()
		a.config.Backup = bk
		a.mu.Unlock()
		if e := a.persistConfig(); e != nil {
			bad(w, "Unable to save rollback snapshot: "+e.Error(), 500)
			return
		}
	}
	changed := []string{}
	for _, entry := range regChoices {
		if v.Profile == "Creator" && (entry.Name == "GameDVR_Enabled" || entry.Name == "AppCaptureEnabled") {
			continue
		} // Creator does not alter recording settings.
		if v.Profile == "Balanced" && (entry.Name == "GameDVR_Enabled" || entry.Name == "AppCaptureEnabled") {
			continue
		}
		if e := regSet(entry); e != nil {
			bad(w, e.Error()+". Use Restore to undo applied settings.", 500)
			return
		}
		changed = append(changed, entry.Name)
	}
	if v.Power {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		out, err := run(ctx, "powercfg.exe", "/setactive", "SCHEME_MIN")
		cancel()
		if err != nil {
			a.logger("High performance plan unavailable: " + short(out, 120))
			changed = append(changed, "High performance plan unavailable")
		} else {
			changed = append(changed, "High performance power plan")
		}
	}
	a.mu.Lock()
	a.config.Preset = v.Profile
	a.mu.Unlock()
	_ = a.persistConfig()
	a.logger("Applied " + v.Profile + " preset")
	send(w, map[string]interface{}{"ok": true, "changed": changed, "profile": v.Profile, "message": "Preset applied. Restart affected games to evaluate results."})
}
func (a *App) restoreHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		bad(w, "POST required", 405)
		return
	}
	var confirm struct {
		AckGameRunning bool `json:"ack_game_running"`
	}
	if e := decode(r, &confirm); e != nil {
		bad(w, e.Error(), 400)
		return
	}
	if e := a.changeDuringGameWarning(confirm.AckGameRunning); e != nil {
		bad(w, e.Error(), 409)
		return
	}
	a.mu.RLock()
	b := a.config.Backup
	a.mu.RUnlock()
	if b == nil {
		bad(w, "No prior changes to restore", 400)
		return
	}
	failed := []string{}
	for _, entry := range b.Entries {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		var out string
		var err error
		if !entry.Exists {
			out, err = run(ctx, "reg.exe", "delete", entry.Key, "/v", entry.Name, "/f")
			if strings.Contains(strings.ToLower(out), "unable to find") {
				err = nil
			}
		} else {
			out, err = run(ctx, "reg.exe", "add", entry.Key, "/v", entry.Name, "/t", entry.Kind, "/d", entry.Data, "/f")
		}
		cancel()
		if err != nil {
			failed = append(failed, entry.Name+": "+short(out, 90))
		}
	}
	if b.PowerGUID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		out, err := run(ctx, "powercfg.exe", "/setactive", b.PowerGUID)
		cancel()
		if err != nil {
			failed = append(failed, "Power plan: "+short(out, 100))
		}
	}
	if len(failed) > 0 {
		bad(w, "Some settings could not be restored: "+strings.Join(failed, "; "), 500)
		return
	}
	a.mu.Lock()
	a.config.Backup = nil
	a.config.Preset = "Stock"
	a.mu.Unlock()
	_ = a.persistConfig()
	a.logger("Original Windows settings restored")
	send(w, map[string]bool{"ok": true})
}

type reqTool struct {
	Name string `json:"name"`
}

func (a *App) toolHandler(w http.ResponseWriter, r *http.Request) {
	var v reqTool
	if e := decode(r, &v); e != nil {
		bad(w, e.Error(), 400)
		return
	}
	var cmd *exec.Cmd
	switch v.Name {
	case "mouse":
		cmd = exec.Command("control.exe", "main.cpl")
	case "keyboard":
		cmd = exec.Command("control.exe", "keyboard")
	case "controller":
		cmd = exec.Command("control.exe", "joy.cpl")
	case "sound":
		cmd = exec.Command("control.exe", "mmsys.cpl")
	case "display":
		cmd = exec.Command("explorer.exe", "ms-settings:display")
	case "graphics":
		cmd = exec.Command("explorer.exe", "ms-settings:display-advancedgraphics")
	case "gamebar":
		cmd = exec.Command("explorer.exe", "ms-settings:gaming-gamebar")
	case "drivers":
		cmd = exec.Command("explorer.exe", "ms-settings:windowsupdate-optionalupdates")
	case "network":
		cmd = exec.Command("explorer.exe", "ms-settings:network")
	case "startup":
		cmd = exec.Command("taskmgr.exe")
	case "nvidia":
		cmd = exec.Command("control.exe", "/name", "NVIDIA.Display")
	case "eventviewer":
		cmd = exec.Command("mmc.exe", "eventvwr.msc")
	case "device":
		cmd = exec.Command("mmc.exe", "devmgmt.msc")
	case "power":
		cmd = exec.Command("control.exe", "powercfg.cpl")
	case "folder":
		cmd = exec.Command("explorer.exe", a.root)
	case "debloat":
		cmd = exec.Command("explorer.exe", "https://github.com/Raphire/Win11Debloat")
	case "presentmon":
		cmd = exec.Command("explorer.exe", "https://github.com/GameTechDev/PresentMon/releases")
	case "amd":
		cmd = exec.Command("explorer.exe", "https://www.amd.com/en/support/download/drivers.html")
	case "nvidiadrivers":
		cmd = exec.Command("explorer.exe", "https://www.nvidia.com/en-us/drivers/")
	default:
		bad(w, "Unknown tool", 400)
		return
	}
	if e := cmd.Start(); e != nil {
		bad(w, e.Error(), 500)
		return
	}
	send(w, map[string]bool{"ok": true})
}

type reqDiagnostic struct {
	AckGameRunning bool   `json:"ack_game_running"`
	Kind           string `json:"kind"`
}

func (a *App) diagnosticHandler(w http.ResponseWriter, r *http.Request) {
	var req reqDiagnostic
	if err := decode(r, &req); err != nil {
		bad(w, err.Error(), 400)
		return
	}
	if e := a.changeDuringGameWarning(req.AckGameRunning); e != nil {
		bad(w, e.Error(), 409)
		return
	}
	var exe string
	var args []string
	var ps string
	switch req.Kind {
	case "sfc":
		exe = "sfc.exe"
		args = []string{"/scannow"}
	case "dism":
		exe = "DISM.exe"
		args = []string{"/Online", "/Cleanup-Image", "/RestoreHealth"}
	case "dismcheck":
		exe = "DISM.exe"
		args = []string{"/Online", "/Cleanup-Image", "/ScanHealth"}
	case "disk":
		exe = "chkdsk.exe"
		args = []string{os.Getenv("SystemDrive") + `\`, "/scan"}
	case "dns":
		exe = "ipconfig.exe"
		args = []string{"/flushdns"}
	case "ipconfig":
		exe = "ipconfig.exe"
		args = []string{"/all"}
	case "drivers":
		exe = "driverquery.exe"
		args = []string{"/v", "/fo", "table"}
	case "events":
		ps = `$ErrorActionPreference='SilentlyContinue'; Get-WinEvent -FilterHashtable @{LogName='System';Id=41,1001,17,18,19,4101;StartTime=(Get-Date).AddDays(-7)} -MaxEvents 35 | Select-Object TimeCreated,Id,ProviderName,LevelDisplayName,@{Name='Message';Expression={$_.Message.Substring(0,[Math]::Min(180,$_.Message.Length))}} | Format-Table -Wrap | Out-String -Width 200`
	case "netadapters":
		ps = `Get-NetAdapter | Select-Object Name,InterfaceDescription,Status,LinkSpeed | Format-Table -AutoSize | Out-String -Width 180`
	case "processes":
		ps = `Get-Process | Sort-Object WorkingSet64 -Descending | Select-Object -First 30 ProcessName,Id,@{N='RAM_MB';E={[math]::Round($_.WorkingSet64/1MB)}},CPU | Format-Table -AutoSize | Out-String -Width 180`
	default:
		bad(w, "Unknown diagnostic", 400)
		return
	}
	id := a.startJob(req.Kind)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
		defer cancel()
		var out string
		var err error
		if ps != "" {
			out, err = powershell(ctx, ps)
		} else {
			out, err = run(ctx, exe, args...)
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			err = ctx.Err()
		}
		a.completeJob(id, out, err)
	}()
	send(w, map[string]string{"ok": "true", "jobID": id})
}
func (a *App) startJob(kind string) string {
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	a.mu.Lock()
	a.jobs = append([]Job{{ID: id, Kind: kind, Started: time.Now().Format("15:04:05"), Status: "Running", Output: "Running Windows diagnostic…"}}, a.jobs...)
	if len(a.jobs) > 30 {
		a.jobs = a.jobs[:30]
	}
	a.mu.Unlock()
	a.logger("Started: " + kind)
	return id
}
func (a *App) completeJob(id, out string, e error) {
	a.mu.Lock()
	for i := range a.jobs {
		if a.jobs[i].ID == id {
			a.jobs[i].Status = "Completed"
			if e != nil {
				a.jobs[i].Status = "Error"
				out = out + "\n\nERROR: " + e.Error()
			}
			a.jobs[i].Output = short(out, 30000)
			a.jobs[i].Ended = time.Now().Format("15:04:05")
			break
		}
	}
	a.mu.Unlock()
	a.logger("Diagnostic finished: " + id)
}

var safeHost = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,250}$`)

type reqPing struct {
	Host string `json:"host"`
}

func (a *App) pingHandler(w http.ResponseWriter, r *http.Request) {
	var v reqPing
	if err := decode(r, &v); err != nil {
		bad(w, err.Error(), 400)
		return
	}
	v.Host = strings.TrimSpace(v.Host)
	if !safeHost.MatchString(v.Host) {
		bad(w, "Enter a hostname or IPv4 address, without spaces or commands", 400)
		return
	}
	id := a.startJob("ping " + v.Host)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
		defer cancel()
		out, e := run(ctx, "ping.exe", "-n", "4", "-w", "1500", v.Host)
		a.completeJob(id, out, e)
	}()
	send(w, map[string]string{"jobID": id})
}

type reqMouse struct {
	Speed int `json:"speed"`
}

func (a *App) mouseHandler(w http.ResponseWriter, r *http.Request) {
	var v reqMouse
	if e := decode(r, &v); e != nil {
		bad(w, e.Error(), 400)
		return
	}
	if v.Speed < 1 || v.Speed > 20 {
		bad(w, "Windows pointer speed must be 1–20", 400)
		return
	}
	if e := setSystemMouseSpeed(v.Speed); e != nil {
		bad(w, "Windows could not change pointer speed: "+e.Error(), 500)
		return
	}
	a.mu.Lock()
	a.hardware["mouseSpeed"] = v.Speed
	a.mu.Unlock()
	send(w, map[string]interface{}{"ok": true, "mouseSpeed": v.Speed})
}

type reqSettings struct {
	PresentMonPath string `json:"presentMonPath"`
}

func (a *App) settingsHandler(w http.ResponseWriter, r *http.Request) {
	var v reqSettings
	if e := decode(r, &v); e != nil {
		bad(w, e.Error(), 400)
		return
	}
	v.PresentMonPath = strings.TrimSpace(strings.Trim(v.PresentMonPath, `"`))
	if v.PresentMonPath != "" && !fileExists(v.PresentMonPath) {
		bad(w, "PresentMon executable was not found at that path", 400)
		return
	}
	if v.PresentMonPath != "" && !strings.EqualFold(filepath.Ext(v.PresentMonPath), ".exe") {
		bad(w, "Choose the official PresentMon console executable (.exe)", 400)
		return
	}
	a.mu.Lock()
	a.config.PresentMonPath = v.PresentMonPath
	a.mu.Unlock()
	_ = a.persistConfig()
	send(w, map[string]bool{"ok": true})
}

func findPresentMon(configured string) string {
	if configured != "" && fileExists(configured) {
		return configured
	}
	if p, e := exec.LookPath("PresentMon.exe"); e == nil {
		return p
	}
	candidates := []string{}
	for _, p := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")} {
		if p != "" {
			candidates = append(candidates, filepath.Join(p, "Intel", "PresentMon", "PresentMon.exe"), filepath.Join(p, "PresentMon", "PresentMon.exe"), filepath.Join(p, "Programs", "PresentMon", "PresentMon.exe"))
		}
	}
	for _, p := range candidates {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

type reqCapture struct {
	GameID      string `json:"gameID"`
	Seconds     int    `json:"seconds"`
	Label       string `json:"label"`
	ProcessName string `json:"processName"`
}

func (a *App) captureHandler(w http.ResponseWriter, r *http.Request) {
	var v reqCapture
	if e := decode(r, &v); e != nil {
		bad(w, e.Error(), 400)
		return
	}
	if v.Seconds < 10 || v.Seconds > 240 {
		bad(w, "Capture duration must be 10–240 seconds", 400)
		return
	}
	a.mu.RLock()
	p := findPresentMon(a.config.PresentMonPath)
	busy := a.capturing
	profile := a.config.Preset
	a.mu.RUnlock()
	if p == "" {
		bad(w, "PresentMon console executable not found. Set its path in Benchmark.", 400)
		return
	}
	if busy {
		bad(w, "A benchmark is already running", 409)
		return
	}
	g, ok := a.lookupGame(v.GameID)
	if !ok {
		bad(w, "Select an installed game first", 400)
		return
	}
	proc := strings.TrimSpace(v.ProcessName)
	if proc == "" {
		proc = filepath.Base(g.Exe)
	}
	if !regexp.MustCompile(`(?i)^[a-z0-9_ .()-]+\.exe$`).MatchString(proc) {
		bad(w, "Enter the real process name from Task Manager, e.g. RainbowSix.exe", 400)
		return
	}
	if strings.Contains(proc, `\`) || strings.Contains(proc, "/") {
		bad(w, "Enter only the exe name, not a path", 400)
		return
	}
	file := filepath.Join(a.root, "captures", fmt.Sprintf("%d_%s.csv", time.Now().Unix(), g.ID))
	if e := os.MkdirAll(filepath.Dir(file), 0755); e != nil {
		bad(w, e.Error(), 500)
		return
	}
	a.mu.Lock()
	a.capturing = true
	a.mu.Unlock()
	id := a.startJob("Frame capture: " + g.Name)
	go func() {
		defer func() { a.mu.Lock(); a.capturing = false; a.mu.Unlock() }()
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(v.Seconds+40)*time.Second)
		defer cancel()
		out, e := run(ctx, p, "--process_name", proc, "--output_file", file, "--timed", strconv.Itoa(v.Seconds), "--terminate_after_timed", "--v1_metrics")
		if e != nil {
			a.completeJob(id, out, e)
			return
		}
		f, e := os.Open(file)
		if e != nil {
			a.completeJob(id, "PresentMon did not produce a CSV. Ensure the game was running and permissions allow ETW tracing.\n"+out, e)
			return
		}
		defer f.Close()
		bm, e := parseBenchmark(f)
		if e != nil {
			a.completeJob(id, "Capture returned no usable frame times. Check the game's actual process name and any capture permissions.\n"+out, e)
			return
		}
		bm.ID = fmt.Sprintf("%d", time.Now().UnixNano())
		bm.Game = g.Name
		bm.Label = short(strings.TrimSpace(v.Label), 80)
		if bm.Label == "" {
			bm.Label = "PresentMon capture"
		}
		bm.Profile = profile
		bm.Date = time.Now().Format("2006-01-02 15:04")
		bm.Source = "PresentMon"
		a.saveBenchmark(bm)
		a.completeJob(id, fmt.Sprintf("Capture finished: %d frames, %.1f average FPS, %.1f 1%% low FPS, %.1f 0.1%% low FPS.\nStored under %s", bm.Frames, bm.AvgFPS, bm.Low1, bm.Low01, file), nil)
	}()
	send(w, map[string]interface{}{"ok": true, "jobID": id, "process": proc})
}
func (a *App) benchmarkHandler(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	send(w, map[string]interface{}{"history": a.history, "presentMon": findPresentMon(a.config.PresentMonPath), "capturing": a.capturing})
}
func (a *App) importHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		bad(w, "POST required", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 65<<20)
	if e := r.ParseMultipartForm(2 << 20); e != nil {
		bad(w, "CSV too large or invalid multipart form", 400)
		return
	}
	file, header, e := r.FormFile("csv")
	if e != nil {
		bad(w, "Choose a PresentMon CSV file", 400)
		return
	}
	defer file.Close()
	defer r.MultipartForm.RemoveAll()
	b, e := parseBenchmark(io.LimitReader(file, 64<<20))
	if e != nil {
		bad(w, e.Error(), 400)
		return
	}
	name := strings.TrimSpace(r.FormValue("game"))
	if name == "" {
		name = "Imported game"
	}
	label := strings.TrimSpace(r.FormValue("label"))
	if label == "" {
		label = header.Filename
	}
	a.mu.RLock()
	preset := a.config.Preset
	a.mu.RUnlock()
	b.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	b.Game = short(name, 90)
	b.Label = short(label, 90)
	b.Profile = preset
	b.Date = time.Now().Format("2006-01-02 15:04")
	b.Source = "CSV import"
	a.saveBenchmark(b)
	send(w, map[string]interface{}{"ok": true, "benchmark": b})
}
func (a *App) saveBenchmark(b Benchmark) {
	a.mu.Lock()
	a.history = append([]Benchmark{b}, a.history...)
	if len(a.history) > 120 {
		a.history = a.history[:120]
	}
	a.mu.Unlock()
	_ = a.persistHistory()
	a.logger("Benchmark saved: " + b.Game)
}
func parseBenchmark(r io.Reader) (Benchmark, error) {
	csvr := csv.NewReader(r)
	csvr.FieldsPerRecord = -1
	csvr.LazyQuotes = true
	headers, e := csvr.Read()
	if e != nil {
		return Benchmark{}, fmt.Errorf("Could not read CSV headers: %w", e)
	}
	columns := map[string]int{}
	for i, h := range headers {
		columns[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\uFEFF")))] = i
	}
	timeNames := []string{"msbetweenpresents", "msbetweenapppresents", "msbetweendisplaychange", "frametime", "frame time (ms)"}
	col := -1
	for _, name := range timeNames {
		if i, ok := columns[name]; ok {
			col = i
			break
		}
	}
	if col < 0 {
		return Benchmark{}, errors.New("No supported frame-time column found. Export a PresentMon CSV with MsBetweenPresents or MsBetweenAppPresents. Found: " + short(strings.Join(headers, ", "), 180))
	}
	values := make([]float64, 0, 14000)
	for len(values) < 200000 {
		record, err := csvr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			continue
		}
		if col >= len(record) {
			continue
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(record[col]), 64)
		if err == nil && f >= 0.1 && f <= 2000 && !math.IsNaN(f) && !math.IsInf(f, 0) {
			values = append(values, f)
		}
	}
	if len(values) < 40 {
		return Benchmark{}, fmt.Errorf("Only %d valid frame times. Need at least 40 frames to calculate meaningful percentiles", len(values))
	}
	return benchmarkStats(values), nil
}
func benchmarkStats(v []float64) Benchmark {
	sorted := append([]float64(nil), v...)
	sort.Float64s(sorted)
	n := len(v)
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	worst := func(frac float64) float64 {
		count := int(math.Ceil(float64(n) * frac))
		if count < 1 {
			count = 1
		}
		total := 0.0
		for i := n - count; i < n; i++ {
			total += sorted[i]
		}
		return 1000 / (total / float64(count))
	}
	at := func(p float64) float64 { i := int(math.Round((float64(n) - 1) * p)); return sorted[i] }
	median := at(.5)
	limit := math.Max(50, 2.0*median)
	stutters := 0
	for _, f := range v {
		if f > limit {
			stutters++
		}
	}
	chart := []float64{}
	step := int(math.Ceil(float64(n) / 200.0))
	if step < 1 {
		step = 1
	}
	for i := 0; i < n; i += step {
		end := i + step
		if end > n {
			end = n
		}
		maxF := v[i]
		for _, x := range v[i:end] {
			if x > maxF {
				maxF = x
			}
		}
		chart = append(chart, math.Round(maxF*100)/100)
	}
	round := func(x float64) float64 { return math.Round(x*10) / 10 }
	return Benchmark{Frames: n, AvgFPS: round(1000 / (sum / float64(n))), Low1: round(worst(.01)), Low01: round(worst(.001)), P99ms: round(at(.99)), Medianms: round(median), Stutters: stutters, Chart: chart}
}
