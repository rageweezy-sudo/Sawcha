//go:build windows

package main

// Native Win32 UI. No browser runtime, Electron, JavaScript, Node, or WebView.
// Uses only Windows user32/gdi32 APIs; the existing Go engine supplies diagnostics.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	u32                = syscall.NewLazyDLL("user32.dll")
	gdi                = syscall.NewLazyDLL("gdi32.dll")
	kern               = syscall.NewLazyDLL("kernel32.dll")
	shell              = syscall.NewLazyDLL("shell32.dll")
	registerClass      = u32.NewProc("RegisterClassExW")
	createWindow       = u32.NewProc("CreateWindowExW")
	defWindowProc      = u32.NewProc("DefWindowProcW")
	getMessage         = u32.NewProc("GetMessageW")
	translateMessage   = u32.NewProc("TranslateMessage")
	dispatchMessage    = u32.NewProc("DispatchMessageW")
	postQuitMessage    = u32.NewProc("PostQuitMessage")
	postMessage        = u32.NewProc("PostMessageW")
	showWindow         = u32.NewProc("ShowWindow")
	updateWindow       = u32.NewProc("UpdateWindow")
	invalidateRect     = u32.NewProc("InvalidateRect")
	setTimer           = u32.NewProc("SetTimer")
	killTimer          = u32.NewProc("KillTimer")
	getClientRect      = u32.NewProc("GetClientRect")
	beginPaint         = u32.NewProc("BeginPaint")
	endPaint           = u32.NewProc("EndPaint")
	fillRect           = u32.NewProc("FillRect")
	setTextColor       = gdi.NewProc("SetTextColor")
	setBkColor         = gdi.NewProc("SetBkColor")
	setBkMode          = gdi.NewProc("SetBkMode")
	drawText           = u32.NewProc("DrawTextW")
	createFont         = gdi.NewProc("CreateFontW")
	createBrush        = gdi.NewProc("CreateSolidBrush")
	deleteObject       = gdi.NewProc("DeleteObject")
	selectObject       = gdi.NewProc("SelectObject")
	createCompatDC     = gdi.NewProc("CreateCompatibleDC")
	createCompatBitmap = gdi.NewProc("CreateCompatibleBitmap")
	deleteDC           = gdi.NewProc("DeleteDC")
	bitBlt             = gdi.NewProc("BitBlt")
	setWindowPos       = u32.NewProc("SetWindowPos")
	sendMessage        = u32.NewProc("SendMessageW")
	getWindowText      = u32.NewProc("GetWindowTextW")
	setWindowText      = u32.NewProc("SetWindowTextW")
	messageBox         = u32.NewProc("MessageBoxW")
	loadCursor         = u32.NewProc("LoadCursorW")
	shellExecute       = shell.NewProc("ShellExecuteW")
	getModuleHandle    = kern.NewProc("GetModuleHandleW")
	getStockObject     = gdi.NewProc("GetStockObject")
)

const (
	wmCreate           = 1
	wmDestroy          = 2
	wmSize             = 5
	wmPaint            = 15
	wmCommand          = 0x111
	wmTimer            = 0x113
	wmLbuttonDown      = 0x201
	wmMouseWheel       = 0x20A
	wmCtlColorEdit     = 0x133
	wmCtlColorStatic   = 0x138
	wmAppRefresh       = 0x8001
	wsOverlappedWindow = 0x00CF0000
	wsChild            = 0x40000000
	wsVisible          = 0x10000000
	wsBorder           = 0x00800000
	wsClipChildren     = 0x02000000
	esAutoHscroll      = 0x80
	esPassword         = 0x20
	esMultiline        = 0x4
	esAutoVscroll      = 0x40
	swShow             = 5
	swHide             = 0
	swpNoZOrder        = 4
	swpNoActivate      = 16
	dtWordBreak        = 0x10
	dtNoPrefix         = 0x800
	dtSingleLine       = 0x20
	dtVCenter          = 4
	dtEndEllipsis      = 0x8000
	mbYesNo            = 4
	mbIconWarning      = 0x30
	mbIconInfo         = 0x40
	idYes              = 6
	wmSetFont          = 0x30
	transparent        = 1
	srcCopy            = 0xCC0020
)

type winPoint struct{ X, Y int32 }
type winRect struct{ L, T, R, B int32 }
type winPaint struct {
	HDC      uintptr
	Erase    int32
	R        winRect
	Restore  int32
	Update   int32
	Reserved [32]byte
}
type winMsg struct {
	HWND           uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	PT             winPoint
	Private        uint32
}
type winClass struct {
	Size                               uint32
	Style                              uint32
	WndProc                            uintptr
	Extra, WndExtra                    int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	IconSmall                          uintptr
}
type hitBox struct {
	r      winRect
	action string
}

func wide(s string) *uint16      { return syscall.StringToUTF16Ptr(s) }
func rgb(r, g, b uint32) uintptr { return uintptr(r | (g << 8) | (b << 16)) }

var (
	bg         = rgb(38, 39, 43)
	navBg      = rgb(31, 32, 36)
	cardBg     = rgb(49, 50, 56)
	cardAccent = rgb(59, 60, 67)
	red        = rgb(247, 70, 81)
	white      = rgb(239, 239, 244)
	muted      = rgb(170, 170, 181)
	green      = rgb(94, 210, 157)
	darker     = rgb(28, 28, 33)
)

var menus = []struct{ id, name, detail string }{
	{"overview", "DASHBOARD", "Your system at a glance"},
	{"audio", "AUDIO STUDIO", "18-band EQ / selective setup"},
	{"plus", "PLUS MEMBERSHIP", "Secure Stripe subscription"},
	{"live", "LIVE MONITOR", "Real resource usage"},
	{"games", "GAME LIBRARY", "Launch & profiles"},
	{"profiles", "OPTIMIZE", "Reversible presets"},
	{"benchmark", "FRAME TIME LAB", "Measure game smoothness"},
	{"input", "DPI + SENSITIVITY", "Per-game mouse profiles"},
	{"keybinds", "KEYBIND GUIDE", "Keyboard + controller"},
	{"assistant", "AI TROUBLESHOOT", "Ask about performance"},
	{"network", "NETWORK MANAGER", "Ping & connectivity"},
	{"repairs", "REPAIR LAB", "Safe diagnostic tools"},
	{"explain", "WHAT DOES IT DO?", "Plain English explanations"},
	{"developer", "DEVELOPER MODE", "Protected diagnostics"},
	{"settings", "SETTINGS", "Quiet gaming / safety / drivers"},
}

type nativeApp struct {
	app                                          *App
	hwnd                                         uintptr
	mu                                           sync.Mutex
	hits                                         []hitBox
	controls                                     map[string]uintptr
	editorBrush                                  uintptr
	font, fontBold, fontSmall, fontBig, fontHero uintptr
	page                                         string
	logged                                       bool
	identity                                     string
	developer                                    bool
	selected                                     int
	scroll                                       int
	status                                       string
	answer                                       string
	googleFlow                                   string
	googleBusy                                   bool
	busy                                         bool
	client                                       *http.Client
	offset                                       int
	plus                                         plusSettings
	timerQuiet                                   bool
}

var native *nativeApp

func makeFont(size int32, bold bool) uintptr {
	weight := uintptr(400)
	if bold {
		weight = 700
	}
	p, _, _ := createFont.Call(uintptr(uint32(-size)), 0, 0, 0, weight, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(wide("Segoe UI"))))
	return p
}
func (n *nativeApp) openLink(link string) {
	shellExecute.Call(0, uintptr(unsafe.Pointer(wide("open"))), uintptr(unsafe.Pointer(wide(link))), 0, 0, uintptr(swShow))
}
func (n *nativeApp) note(s string) { n.mu.Lock(); n.status = s; n.mu.Unlock(); n.invalidate() }
func (n *nativeApp) invalidate() {
	if n.hwnd != 0 {
		postMessage.Call(n.hwnd, wmAppRefresh, 0, 0)
	}
}
func (n *nativeApp) api(path string, data interface{}) (map[string]interface{}, error) {
	b, _ := json.Marshal(data)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://127.0.0.1:%d%s", n.app.port, path), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Sawcha-Token", n.app.token)
	resp, err := n.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var out map[string]interface{}
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("unexpected API response: %s", short(string(raw), 160))
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s", out["error"])
	}
	return out, nil
}
func (n *nativeApp) async(fn func() error) {
	n.mu.Lock()
	if n.busy {
		n.mu.Unlock()
		n.note("Another task is running.")
		return
	}
	n.busy = true
	n.mu.Unlock()
	n.invalidate()
	go func() {
		err := fn()
		n.mu.Lock()
		n.busy = false
		n.mu.Unlock()
		if err != nil {
			n.note("ERROR: " + err.Error())
		} else {
			n.invalidate()
		}
	}()
}
func (n *nativeApp) read(id string) string {
	h := n.controls[id]
	if h == 0 {
		return ""
	}
	buf := make([]uint16, 8192)
	k, _, _ := getWindowText.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if k == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:k])
}
func (n *nativeApp) put(id, value string) {
	if h := n.controls[id]; h != 0 {
		setWindowText.Call(h, uintptr(unsafe.Pointer(wide(value))))
	}
}
func (n *nativeApp) pageTo(s string) {
	n.mu.Lock()
	n.page = s
	n.scroll = 0
	n.mu.Unlock()
	n.layout()
	n.invalidate()
}
func (n *nativeApp) confirm(s string) bool {
	p, _, _ := messageBox.Call(n.hwnd, uintptr(unsafe.Pointer(wide(s))), uintptr(unsafe.Pointer(wide("SAWCHA // OPTIMIZER"))), mbYesNo|mbIconWarning)
	return p == idYes
}

// A visible warning is shown before configuration changes made while playing.
func (n *nativeApp) gameWarning(action string) bool {
	game, prefs := n.app.guardSnapshot()
	if game == "" || !prefs.WarnBeforeChanges {
		return true
	}
	return n.confirm("GAME RUNNING: " + game + "\n\n" + action + " could cause stutter or interruptions while playing.\n\nSave your game or wait until the match ends. Continue anyway?")
}

func (n *nativeApp) act(key string) {
	if strings.HasPrefix(key, "audio:") {
		n.audioAction(strings.TrimPrefix(key, "audio:"))
		return
	}
	if strings.HasPrefix(key, "nav:") {
		n.pageTo(strings.TrimPrefix(key, "nav:"))
		return
	}
	if strings.HasPrefix(key, "choose:") {
		v, _ := strconv.Atoi(strings.TrimPrefix(key, "choose:"))
		n.selected = v
		n.invalidate()
		return
	}
	if strings.HasPrefix(key, "pref:") {
		field := strings.TrimPrefix(key, "pref:")
		if field == "latency" && n.app.preferencesSnapshot().LatencySafe {
			if !n.confirm("Disable strict latency protection? This permits speculative Windows registry and power-plan changes. Measure before and after, and use RESTORE if frame pacing becomes worse.") { return }
		}
		n.async(func() error {
			_, p := n.app.guardSnapshot()
			switch field {
			case "detect":
				p.DetectGames = !p.DetectGames
			case "quiet":
				p.QuietWhileGaming = !p.QuietWhileGaming
			case "warn":
				p.WarnBeforeChanges = !p.WarnBeforeChanges
			case "latency":
				p.LatencySafe = !p.LatencySafe
			case "interval":
				if p.QuietMonitorSeconds == 60 {
					p.QuietMonitorSeconds = 90
				} else if p.QuietMonitorSeconds == 90 {
					p.QuietMonitorSeconds = 120
				} else {
					p.QuietMonitorSeconds = 60
				}
			case "defaults":
				p = defaultAppPreferences()
			default:
				return errors.New("Unknown setting")
			}
			_, e := n.api("/api/preferences", p)
			if e == nil {
				n.note("Preferences saved. Sawcha will respect your updated game-safety settings.")
			}
			return e
		})
		return
	}
	if strings.HasPrefix(key, "tool:") {
		what := strings.TrimPrefix(key, "tool:")
		n.async(func() error {
			_, e := n.api("/api/tool", map[string]string{"name": what})
			if e == nil {
				n.note("Opened Windows " + what + " controls.")
			}
			return e
		})
		return
	}
	if strings.HasPrefix(key, "repair:") {
		what := strings.TrimPrefix(key, "repair:")
		if !n.confirm(repairMessage(what)) || !n.gameWarning("Starting this diagnostic") {
			return
		}
		n.async(func() error {
			v, e := n.api("/api/diagnostic", map[string]interface{}{"kind": what, "ack_game_running": true})
			if e == nil {
				n.note("Started " + what + " job. Watch job history below. ID: " + fmt.Sprint(v["jobID"]))
			}
			return e
		})
		return
	}
	if strings.HasPrefix(key, "preset:") {
		what := strings.TrimPrefix(key, "preset:")
		if !n.confirm(presetExplanation(what)+"\n\nApply now? A backup will be saved first.") || !n.gameWarning("Applying a Windows optimization preset") {
			return
		}
		n.async(func() error {
			_, e := n.api("/api/preset", map[string]interface{}{"profile": what, "power": false, "ack_game_running": true})
			if e == nil {
				n.note("Applied " + what + "; restart your game and compare benchmarks.")
			}
			return e
		})
		return
	}
	switch key {
	case "pluscheckout":
		server, email := strings.TrimSpace(n.read("plusurl")), strings.TrimSpace(n.read("plusemail"))
		n.async(func() error {
			n.mu.Lock()
			p := n.plus
			n.mu.Unlock()
			p.Server = server
			p.Email = email
			link, e := plusCheckout(context.Background(), &p)
			if e != nil {
				return e
			}
			n.mu.Lock()
			n.plus = p
			n.mu.Unlock()
			n.openLink(link)
			n.note("Stripe Checkout opened. Return here and select CHECK PURCHASE STATUS after payment.")
			return nil
		})
	case "pluspoll":
		n.async(func() error {
			n.mu.Lock()
			p := n.plus
			n.mu.Unlock()
			active, e := plusPoll(context.Background(), &p)
			if e != nil {
				return e
			}
			n.mu.Lock()
			n.plus = p
			n.mu.Unlock()
			if active {
				n.note("SAWCHA PLUS is active on this installation.")
			} else {
				n.note("Plus not confirmed yet: " + p.Message)
			}
			return nil
		})
	case "plusactivate":
		server, keyvalue := strings.TrimSpace(n.read("plusurl")), strings.TrimSpace(n.read("pluskey"))
		n.async(func() error {
			n.mu.Lock()
			p := n.plus
			n.mu.Unlock()
			p.Server = server
			p.LicenseKey = keyvalue
			active, e := plusActivate(context.Background(), &p)
			if e != nil {
				return e
			}
			if !active {
				return errors.New("Plus key was not accepted by the licensing server")
			}
			if e = savePlus(p); e != nil {
				return e
			}
			n.mu.Lock()
			n.plus = p
			n.mu.Unlock()
			n.put("pluskey", "")
			n.note("SAWCHA PLUS activated successfully.")
			return nil
		})
	case "plusverify":
		n.async(func() error {
			n.mu.Lock()
			p := n.plus
			n.mu.Unlock()
			active, e := plusVerify(context.Background(), &p)
			n.mu.Lock()
			n.plus = p
			n.mu.Unlock()
			if e != nil {
				return e
			}
			if !active {
				n.note("Plus inactive; Basic tools remain available.")
			} else {
				n.note("Plus verified with server. Valid until " + p.Expires)
			}
			return nil
		})
	case "plusmanage":
		n.async(func() error {
			n.mu.Lock()
			p := n.plus
			n.mu.Unlock()
			link, e := plusPortal(context.Background(), p)
			if e != nil {
				return e
			}
			n.openLink(link)
			n.note("Opened secure Stripe subscription portal")
			return nil
		})
	case "guest":
		n.async(func() error {
			_, e := n.api("/api/auth/guest", map[string]string{})
			if e == nil {
				n.mu.Lock()
				n.logged = true
				n.identity = "Local guest"
				n.page = "overview"
				n.mu.Unlock()
				n.layout()
				n.note("Signed in locally. Automatic scan results are ready.")
			}
			return e
		})
	case "google":
		cid := strings.TrimSpace(n.read("googleid"))
		n.async(func() error {
			if cid != "" {
				if _, e := n.api("/api/auth/google/configure", map[string]string{"clientID": cid}); e != nil {
					return e
				}
			}
			v, e := n.api("/api/auth/google/start", map[string]string{})
			if e != nil {
				return e
			}
			flow, url := fmt.Sprint(v["flowID"]), fmt.Sprint(v["url"])
			n.mu.Lock()
			n.googleFlow = flow
			n.googleBusy = true
			n.mu.Unlock()
			n.openLink(url)
			n.note("Complete Google sign-in in your browser, then return here.")
			go n.waitForGoogle(flow)
			return nil
		})
	case "logout":
		n.async(func() error {
			_, e := n.api("/api/auth/logout", map[string]string{})
			if e == nil {
				n.mu.Lock()
				n.logged = false
				n.developer = false
				n.page = "overview"
				n.identity = ""
				n.mu.Unlock()
				n.layout()
				n.note("Signed out.")
			}
			return e
		})
	case "rescan":
		if !n.gameWarning("Running a deep hardware/game scan") {
			return
		}
		n.async(func() error {
			_, e := n.api("/api/scan", map[string]bool{"ack_game_running": true})
			if e == nil {
				n.note("Hardware/game scan started.")
			}
			return e
		})
	case "launch":
		g, ok := n.game()
		if !ok {
			n.note("Select a detected game first.")
			return
		}
		n.async(func() error {
			_, e := n.api("/api/games/launch", map[string]string{"id": g.ID})
			if e == nil {
				n.note("Launched " + g.Name)
			}
			return e
		})
	case "addgame":
		name, path := strings.TrimSpace(n.read("gamename")), strings.TrimSpace(n.read("gamepath"))
		n.async(func() error {
			_, e := n.api("/api/games/add", map[string]string{"name": name, "exe": path})
			if e == nil {
				n.note("Added " + name)
			}
			return e
		})
	case "setexe":
		g, ok := n.game()
		if !ok {
			return
		}
		path := strings.TrimSpace(n.read("gamepath"))
		n.async(func() error {
			_, e := n.api("/api/games/exe", map[string]string{"id": g.ID, "exe": path})
			if e == nil {
				n.note("Game executable path saved.")
			}
			return e
		})
	case "restore":
		if !n.confirm("Restore the Windows registry and power plan values that SAWCHA backed up before applying presets?") || !n.gameWarning("Restoring Windows settings") {
			return
		}
		n.async(func() error {
			_, e := n.api("/api/restore", map[string]bool{"ack_game_running": true})
			if e == nil {
				n.note("Original Windows settings restored.")
			}
			return e
		})
	case "saveinput":
		g, ok := n.game()
		if !ok {
			n.note("Choose a game in Game Library first.")
			return
		}
		dpi, _ := strconv.Atoi(n.read("dpi"))
		sens, _ := strconv.ParseFloat(n.read("sensitivity"), 64)
		poll, _ := strconv.Atoi(n.read("polling"))
		n.async(func() error {
			_, e := n.api("/api/sensitivity", map[string]interface{}{"gameID": g.ID, "dpi": dpi, "sens": sens, "pollingHz": poll})
			if e == nil {
				n.note(fmt.Sprintf("Saved %s: %d DPI, %.3f in-game sensitivity, %d Hz.", g.Name, dpi, sens, poll))
			}
			return e
		})
	case "askoffline", "askonline":
		q := strings.TrimSpace(n.read("question"))
		if len(q) < 2 {
			n.note("Type a question first.")
			return
		}
		g, ok := n.game()
		id := ""
		if ok {
			id = g.ID
		}
		mode := "offline"
		keyval := ""
		if key == "askonline" {
			mode = "online"
			keyval = n.read("apikey")
			if !n.confirm("Online AI sends your question to OpenAI. Hardware data is not shared. Continue?") {
				return
			}
		}
		n.async(func() error {
			v, e := n.api("/api/assistant/chat", map[string]interface{}{"message": q, "gameID": id, "mode": mode, "apiKey": keyval, "includeHardware": false})
			if e == nil {
				n.mu.Lock()
				n.answer = fmt.Sprint(v["answer"])
				n.mu.Unlock()
				n.note("Troubleshooting response ready.")
			}
			return e
		})
	case "ping":
		host := strings.TrimSpace(n.read("hostname"))
		n.async(func() error {
			_, e := n.api("/api/ping", map[string]string{"host": host})
			if e == nil {
				n.note("Ping running; check the network job output below.")
			}
			return e
		})
	case "benchmark", "latencybaseline", "latencyafter":
		g, ok := n.game()
		if !ok {
			n.note("Select a game first.")
			return
		}
		if !n.confirm("Capture 30 seconds using PresentMon? First start your game. PresentMon console must be installed. This tool does not inject into the game.") {
			return
		}
		n.async(func() error {
			label := "Native 30s capture"
			if key == "latencybaseline" || key == "latencyafter" { label = "Latency Guard same scene" }
			_, e := n.api("/api/benchmark/capture", map[string]interface{}{"gameID": g.ID, "seconds": 30, "label": label, "processName": filepath.Base(g.Exe)})
			if e == nil {
				n.note("PresentMon frame-time capture started.")
			}
			return e
		})
	case "latencycompare":
		n.app.mu.RLock()
		history := append([]Benchmark(nil), n.app.history...)
		n.app.mu.RUnlock()
		g, hasGame := n.game()
		matching := []Benchmark{}
		for _, b := range history {
			if b.Label == "Latency Guard same scene" && (!hasGame || strings.EqualFold(b.Game, g.Name)) { matching = append(matching, b) }
			if len(matching) == 2 { break }
		}
		if len(matching) < 2 { n.note("Capture a BASELINE and AFTER run for the same game in the same scene before comparing."); return }
		n.async(func() error {
			report, e := n.api("/api/latency/compare", map[string]string{"beforeID": matching[1].ID, "afterID": matching[0].ID})
			if e == nil { n.note(fmt.Sprint(report["message"])) }
			return e
		})
	case "savepresentmon":
		path := strings.TrimSpace(n.read("presentmon"))
		n.async(func() error {
			_, e := n.api("/api/settings", map[string]string{"presentMonPath": path})
			if e == nil {
				n.note("PresentMon path saved.")
			}
			return e
		})
	case "devsetup", "devunlock":
		pass := n.read("devpass")
		route := "/api/auth/developer/setup"
		if key == "devunlock" {
			route = "/api/auth/developer/unlock"
		}
		n.async(func() error {
			_, e := n.api(route, map[string]string{"password": pass})
			if e == nil {
				if key == "devunlock" {
					n.mu.Lock()
					n.developer = true
					n.mu.Unlock()
				}
				n.put("devpass", "")
				n.note("Developer credential step completed.")
			}
			return e
		})
	case "devlock":
		n.async(func() error {
			_, e := n.api("/api/auth/developer/lock", map[string]string{})
			if e == nil {
				n.mu.Lock()
				n.developer = false
				n.mu.Unlock()
				n.note("Developer mode locked.")
			}
			return e
		})
	case "devreport":
		n.async(func() error {
			v, e := n.api("/api/developer/report", map[string]string{})
			if e != nil {
				return e
			}
			b, _ := json.MarshalIndent(v, "", "  ")
			p := filepath.Join(n.app.root, "developer_report.json")
			if e = os.WriteFile(p, b, 0600); e == nil {
				n.note("Saved report: " + p)
			}
			return e
		})
	case "signing":
		n.openLink("https://learn.microsoft.com/en-us/windows/msix/package/signing-package-overview")
	case "verify":
		n.async(func() error {
			cmd := exec.Command("powershell.exe", "-NoProfile", "-Command", "$s=Get-AuthenticodeSignature '"+strings.ReplaceAll(os.Args[0], "'", "''")+"'; [Console]::WriteLine(('Status: '+$s.Status+' Publisher: '+$s.SignerCertificate.Subject))")
			prepareHidden(cmd)
			b, e := cmd.CombinedOutput()
			n.note(strings.TrimSpace(string(b)))
			return e
		})
	case "openfolder":
		n.async(func() error { _, e := n.api("/api/tool", map[string]string{"name": "folder"}); return e })
	}
}

func (n *nativeApp) game() (Game, bool) {
	n.app.mu.RLock()
	defer n.app.mu.RUnlock()
	if n.selected < 0 || n.selected >= len(n.app.games) {
		return Game{}, false
	}
	return n.app.games[n.selected], true
}
func (n *nativeApp) waitForGoogle(flow string) {
	for i := 0; i < 120; i++ {
		time.Sleep(2 * time.Second)
		v, e := n.api("/api/auth/google/status", map[string]string{"flowID": flow})
		if e != nil {
			n.note("Google sign-in: " + e.Error())
			break
		}
		if x := fmt.Sprint(v["error"]); x != "" && x != "<nil>" {
			n.note("Google sign-in: " + x)
			break
		}
		if v["done"] == true {
			o, e := n.api("/api/auth/google/finish", map[string]string{"flowID": flow})
			if e != nil {
				n.note("Google sign-in: " + e.Error())
			} else {
				n.mu.Lock()
				n.logged = true
				n.page = "overview"
				n.identity = fmt.Sprint(o["identity"])
				n.mu.Unlock()
				n.layout()
				n.note("Google sign-in verified. Developer access remains locked.")
			}
			break
		}
	}
	n.mu.Lock()
	n.googleBusy = false
	n.mu.Unlock()
	n.invalidate()
}

func presetExplanation(kind string) string {
	s := "The changes are reversible. No registry tweak guarantees extra FPS."
	switch kind {
	case "Competitive":
		return "COMPETITIVE: Enables Windows Game Mode and turns off built-in Game DVR background recording (not OBS). It can reduce capture overhead but prevents Game Bar clips while active. " + s
	case "Balanced":
		return "BALANCED: Enables Windows Game Mode. Keeps Game DVR and recording options as they are. " + s
	case "Creator":
		return "CREATOR: Enables Game Mode without disabling built-in capture. Better for streaming and video recording. " + s
	}
	return s
}
func repairMessage(kind string) string {
	switch kind {
	case "sfc":
		return "SFC checks protected Windows system files and repairs damaged copies. It can take several minutes and needs administrator rights."
	case "dism":
		return "DISM checks and repairs the Windows component store. It may use Windows Update and may require administrator rights."
	case "dns":
		return "Flush DNS clears cached domain lookups. It can fix stale DNS entries but will not reduce your internet provider latency."
	case "disk":
		return "CHKDSK /scan checks for file system problems without scheduling a full offline repair."
	case "events":
		return "View system events including crash and GPU reset records. This is read-only."
	case "processes":
		return "List applications consuming the most RAM. This is read-only."
	case "drivers":
		return "List Windows drivers for troubleshooting. This is read-only."
	case "ipconfig":
		return "Show network adapter and IP configuration. This is read-only."
	}
	return "Run this Windows diagnostic task?"
}

func (n *nativeApp) layout() {
	if n.hwnd == 0 {
		return
	}
	var c winRect
	getClientRect.Call(n.hwnd, uintptr(unsafe.Pointer(&c)))
	w := int(c.R)
	x := 280
	usable := w - x - 30
	if usable < 420 {
		usable = 420
	}
	n.mu.Lock()
	page, logged := n.page, n.logged
	n.mu.Unlock()
	positions := map[string]winRect{}
	n.app.mu.RLock()
	ready := n.app.ready
	n.app.mu.RUnlock()
	if !logged && ready {
		positions["googleid"] = winRect{int32(x + 52), 505, int32(x + 52 + usable/2), 543}
	}
	if logged {
		switch page {
		case "audio-setup":
			positions["audiodevice"] = winRect{int32(x + 15), 654, int32(x + usable - 186), 686}
		case "plus":
			positions["plusurl"] = winRect{int32(x + 20), 300, int32(x + usable - 25), 334}
			positions["plusemail"] = winRect{int32(x + 20), 392, int32(x + usable/2), 430}
			positions["pluskey"] = winRect{int32(x + 20), 581, int32(x + usable - 24), 619}
		case "games":
			positions["gamename"] = winRect{int32(x + 20), 608, int32(x + 200), 640}
			positions["gamepath"] = winRect{int32(x + 210), 608, int32(x + usable - 22), 640}
		case "input":
			positions["dpi"] = winRect{int32(x + 20), 314, int32(x + 195), 353}
			positions["sensitivity"] = winRect{int32(x + 208), 314, int32(x + 385), 353}
			positions["polling"] = winRect{int32(x + 398), 314, int32(x + 575), 353}
		case "assistant":
			positions["question"] = winRect{int32(x + 20), 250, int32(x + usable - 25), 287}
			positions["apikey"] = winRect{int32(x + 20), 428, int32(x + usable/2 + 50), 465}
		case "network":
			positions["hostname"] = winRect{int32(x + 20), 272, int32(x + usable - 220), 311}
		case "developer":
			positions["devpass"] = winRect{int32(x + 20), 293, int32(x + usable - 40), 331}
		case "benchmark":
			positions["presentmon"] = winRect{int32(x + 20), 292, int32(x + usable - 40), 331}
		}
	}
	for id, h := range n.controls {
		r, ok := positions[id]
		if !ok {
			showWindow.Call(h, swHide)
			continue
		}
		setWindowPos.Call(h, 0, uintptr(r.L), uintptr(r.T), uintptr(r.R-r.L), uintptr(r.B-r.T), swpNoZOrder|swpNoActivate)
		showWindow.Call(h, swShow)
	}
}

func nativeWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	n := native
	if n == nil {
		v, _, _ := defWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
		return v
	}
	switch msg {
	case wmCreate:
		n.hwnd = hwnd
		n.editorBrush, _, _ = createBrush.Call(cardAccent)
		n.font = makeFont(17, false)
		n.fontSmall = makeFont(14, false)
		n.fontBold = makeFont(19, true)
		n.fontBig = makeFont(31, true)
		n.fontHero = makeFont(53, true)
		editStyle := uintptr(wsChild | wsBorder | esAutoHscroll)
		for _, id := range []string{"googleid", "gamename", "gamepath", "dpi", "sensitivity", "polling", "question", "apikey", "hostname", "devpass", "presentmon", "plusurl", "plusemail", "pluskey", "audiodevice"} {
			style := editStyle
			if id == "devpass" || id == "apikey" {
				style |= esPassword
			}
			h, _, _ := createWindow.Call(0, uintptr(unsafe.Pointer(wide("EDIT"))), uintptr(unsafe.Pointer(wide(""))), style, 0, 0, 100, 32, hwnd, 0, 0, 0)
			n.controls[id] = h
			sendMessage.Call(h, wmSetFont, n.font, 1)
		}
		n.put("dpi", "800")
		n.put("sensitivity", "1.0")
		n.put("polling", "1000")
		n.put("hostname", "1.1.1.1")
		n.put("plusurl", n.plus.Server)
		n.put("plusemail", n.plus.Email)
		n.put("question", "Why does my game stutter even at high FPS?")
		n.put("audiodevice", n.app.config.Audio.DevicePattern)
		n.layout()
		setTimer.Call(hwnd, 1, 1500, 0)
		return 0
	case wmSize:
		n.layout()
		invalidateRect.Call(hwnd, 0, 0)
		return 0
	case wmTimer:
		quiet := n.app.isQuietGaming()
		if quiet != n.timerQuiet {
			n.timerQuiet = quiet
			if quiet {
				// Minimize full-window redraws and CPU scheduling interruptions while gaming.
				setTimer.Call(hwnd, 1, 15000, 0)
			} else {
				setTimer.Call(hwnd, 1, 1500, 0)
			}
		}
		n.layout()
		invalidateRect.Call(hwnd, 0, 0)
		return 0
	case wmAppRefresh:
		n.layout()
		invalidateRect.Call(hwnd, 0, 0)
		return 0
	case wmCtlColorEdit, wmCtlColorStatic:
		setTextColor.Call(wParam, white)
		setBkColor.Call(wParam, cardAccent)
		return n.editorBrush
	case wmMouseWheel:
		amt := int(int16((wParam >> 16) & 0xffff))
		if amt > 0 {
			n.scroll -= 2
		} else {
			n.scroll += 2
		}
		if n.scroll < 0 {
			n.scroll = 0
		}
		if n.scroll > 150 {
			n.scroll = 150
		}
		n.invalidate()
		return 0
	case wmLbuttonDown:
		xx, yy := int32(int16(lParam&0xffff)), int32(int16((lParam>>16)&0xffff))
		n.mu.Lock()
		hits := append([]hitBox(nil), n.hits...)
		n.mu.Unlock()
		for i := len(hits) - 1; i >= 0; i-- {
			r := hits[i].r
			if xx >= r.L && xx < r.R && yy >= r.T && yy < r.B {
				go n.act(hits[i].action)
				return 0
			}
		}
	case wmPaint:
		n.paint()
		return 0
	case wmDestroy:
		killTimer.Call(hwnd, 1)
		for _, p := range []uintptr{n.font, n.fontSmall, n.fontBold, n.fontBig, n.fontHero, n.editorBrush} {
			if p != 0 {
				deleteObject.Call(p)
			}
		}
		postQuitMessage.Call(0)
		return 0
	}
	v, _, _ := defWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return v
}

func (n *nativeApp) rect(dc uintptr, x, y, w, h int, color uintptr) {
	if w < 1 || h < 1 {
		return
	}
	r := winRect{int32(x), int32(y), int32(x + w), int32(y + h)}
	brush, _, _ := createBrush.Call(color)
	fillRect.Call(dc, uintptr(unsafe.Pointer(&r)), brush)
	deleteObject.Call(brush)
}
func (n *nativeApp) text(dc uintptr, val string, x, y, w, h int, color, font uintptr, wrap bool) {
	if val == "" || w < 1 || h < 1 {
		return
	}
	a := syscall.StringToUTF16(val)
	r := winRect{int32(x), int32(y), int32(x + w), int32(y + h)}
	selectObject.Call(dc, font)
	setBkMode.Call(dc, transparent)
	setTextColor.Call(dc, color)
	flags := uintptr(dtNoPrefix | dtEndEllipsis)
	if wrap {
		flags = dtNoPrefix | dtWordBreak
	} else {
		flags |= dtSingleLine | dtVCenter
	}
	drawText.Call(dc, uintptr(unsafe.Pointer(&a[0])), uintptr(len(a)-1), uintptr(unsafe.Pointer(&r)), flags)
}
func (n *nativeApp) button(dc uintptr, action, label string, x, y, w, h int, accent bool) {
	c := cardAccent
	fc := white
	if accent {
		c = red
		fc = darker
	}
	n.rect(dc, x, y, w, h, c)
	n.rect(dc, x, y, w, 2, func() uintptr {
		if accent {
			return white
		}
		return red
	}())
	n.text(dc, label, x+12, y+1, w-24, h-2, fc, n.fontBold, false)
	n.hits = append(n.hits, hitBox{winRect{int32(x), int32(y), int32(x + w), int32(y + h)}, action})
}
func (n *nativeApp) panel(dc uintptr, x, y, w, h int) {
	n.rect(dc, x, y, w, h, cardBg)
	n.rect(dc, x, y, 3, h, red)
}
func (n *nativeApp) line(dc uintptr, x, y, w int) { n.rect(dc, x, y, w, 1, cardAccent) }
func (n *nativeApp) heading(dc uintptr, topic, sub string, x, y, w int) {
	n.text(dc, topic, x, y, w, 44, red, n.fontBig, false)
	n.text(dc, sub, x, y+43, w, 43, muted, n.font, false)
}
func (n *nativeApp) small(dc uintptr, t string, x, y, w int) {
	n.text(dc, t, x, y, w, 30, muted, n.fontSmall, false)
}
func (n *nativeApp) wrap(dc uintptr, t string, x, y, w, h int) {
	n.text(dc, t, x, y, w, h, white, n.font, true)
}
func (n *nativeApp) card(dc uintptr, x, y, w, h int, eyebrow, value, note string) {
	n.panel(dc, x, y, w, h)
	n.small(dc, eyebrow, x+18, y+13, w-30)
	n.text(dc, value, x+18, y+43, w-35, 38, red, n.fontBold, false)
	n.text(dc, note, x+18, y+88, w-34, h-94, muted, n.fontSmall, true)
}

func (n *nativeApp) paint() {
	var ps winPaint
	hdc, _, _ := beginPaint.Call(n.hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer endPaint.Call(n.hwnd, uintptr(unsafe.Pointer(&ps)))
	var cr winRect
	getClientRect.Call(n.hwnd, uintptr(unsafe.Pointer(&cr)))
	w, h := int(cr.R), int(cr.B)
	if w < 800 || h < 600 {
		return
	}
	mem, _, _ := createCompatDC.Call(hdc)
	if mem == 0 {
		return
	}
	defer deleteDC.Call(mem)
	bmp, _, _ := createCompatBitmap.Call(hdc, uintptr(w), uintptr(h))
	if bmp == 0 {
		return
	}
	defer deleteObject.Call(bmp)
	old, _, _ := selectObject.Call(mem, bmp)
	defer selectObject.Call(mem, old)
	n.hits = n.hits[:0]
	n.rect(mem, 0, 0, w, h, bg)
	n.app.mu.RLock()
	ready, stage, progress := n.app.ready, n.app.stage, n.app.progress
	n.app.mu.RUnlock()
	n.mu.Lock()
	logged, page, stat, busy, identity, ans := n.logged, n.page, n.status, n.busy, n.identity, n.answer
	n.mu.Unlock()
	if !ready {
		n.text(mem, "SAWCHA //", 76, 92, w-120, 92, red, n.fontHero, false)
		n.text(mem, "OPTIMIZER", 76, 169, w-120, 72, white, n.fontBig, false)
		n.text(mem, "BUILT BY sawcha     |     NATIVE WINDOWS EDITION", 80, 260, w-130, 35, muted, n.fontBold, false)
		n.panel(mem, 76, 350, w-152, 213)
		n.text(mem, "INITIALIZING YOUR PC", 105, 372, w-214, 44, red, n.fontBig, false)
		n.text(mem, stage, 108, 432, w-214, 33, white, n.font, false)
		n.rect(mem, 108, 489, w-218, 18, darker)
		n.rect(mem, 108, 489, int(float64(w-218)*float64(progress)/100), 18, red)
		n.text(mem, fmt.Sprintf("%d%% COMPLETE  •  STEAM / EPIC / CPU / NVIDIA", progress), 108, 519, w-220, 30, muted, n.fontSmall, false)
		n.text(mem, "Automatic startup hardware, driver and installed-game scan", 80, h-83, w-120, 30, muted, n.font, false)
	} else if !logged {
		n.text(mem, "SAWCHA // OPTIMIZER", 80, 75, w-130, 83, red, n.fontHero, false)
		n.text(mem, "NATIVE WINDOWS EDITION", 84, 163, w-130, 38, white, n.fontBold, false)
		n.text(mem, "Built by sawcha    •    Windows 10 / 11", 84, 205, w-140, 34, muted, n.font, false)
		n.panel(mem, 280, 265, w-560, 365)
		n.text(mem, "WELCOME BACK", 310, 283, w-620, 45, red, n.fontBig, false)
		n.wrap(mem, "Your automatic scan is complete. Choose offline access or sign in securely with Google. A Google account does not grant developer/admin permissions.", 310, 334, w-620, 68)
		n.button(mem, "guest", "CONTINUE OFFLINE", 310, 408, 220, 42, true)
		n.button(mem, "google", "SIGN IN WITH GOOGLE", 545, 408, 278, 42, false)
		n.small(mem, "GOOGLE DESKTOP OAUTH CLIENT ID (optional; set up in Google Cloud)", 310, 474, w-620)
		n.text(mem, "Signing in uses Google's browser page, not an embedded login screen.", 310, 554, w-620, 40, muted, n.fontSmall, true)
		n.text(mem, stat, 80, h-61, w-135, 32, red, n.fontSmall, true)
	} else {
		n.paintApp(mem, w, h, page, identity, ans, stat, busy)
	}
	bitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), mem, 0, 0, srcCopy)
}

func (n *nativeApp) paintApp(dc uintptr, w, h int, page, identity, answer, status string, busy bool) {
	n.rect(dc, 0, 0, 252, h, navBg)
	n.text(dc, "SAWCHA //", 22, 23, 212, 44, red, n.fontBig, false)
	n.text(dc, "OPTIMIZER", 22, 65, 215, 36, white, n.fontBold, false)
	n.small(dc, "BUILT BY sawcha", 22, 108, 215)
	for i, m := range menus {
		yy := 143 + i*36
		if page == m.id {
			n.rect(dc, 8, yy-3, 238, 32, cardAccent)
			n.rect(dc, 8, yy-3, 4, 32, red)
		}
		n.text(dc, m.name, 22, yy+1, 216, 27, func() uintptr {
			if page == m.id {
				return red
			}
			return white
		}(), n.fontSmall, false)
		n.hits = append(n.hits, hitBox{winRect{8, int32(yy - 3), 246, int32(yy + 30)}, "nav:" + m.id})
	}
	n.text(dc, "NATIVE / NO JAVASCRIPT", 22, h-80, 220, 27, green, n.fontSmall, false)
	n.text(dc, "v8.0  •  WINDOWS 10 + 11", 22, h-49, 225, 26, muted, n.fontSmall, false)
	n.rect(dc, 252, 0, w-252, 81, darker)
	game, prefs := n.app.guardSnapshot()
	if game != "" {
		n.text(dc, "GAME RUNNING", 282, 12, 270, 30, red, n.fontBold, false)
		n.text(dc, short(game, 43), 282, 41, 320, 27, white, n.fontSmall, false)
	} else {
		n.text(dc, "SYSTEM ACTIVE", 282, 17, 240, 38, green, n.fontBold, false)
	}
	if game != "" && prefs.QuietWhileGaming {
		n.text(dc, "QUIET MODE • SCANS PAUSED", 630, 20, w-680, 31, green, n.fontSmall, false)
	}
	n.text(dc, "SAWCHA / PERFORMANCE WORKSTATION", 520, 18, w-565, 38, muted, n.fontBold, false)
	n.rect(dc, 252, h-66, w-252, 66, navBg)
	if busy {
		n.text(dc, "WORKING  •  "+short(status, 120), 282, h-59, w-310, 52, red, n.fontSmall, true)
	} else {
		n.text(dc, short(status, 185), 282, h-59, w-310, 52, muted, n.fontSmall, true)
	}
	x := 278
	cw := w - 310
	if cw < 530 {
		cw = 530
	}
	switch page {
	case "plus":
		n.paintPlus(dc, x, cw)
	case "overview":
		n.paintOverview(dc, x, cw)
	case "audio":
		n.paintAudioStudio(dc, x, cw)
	case "audio-setup":
		n.paintAudioSetup(dc, x, cw)
	case "live":
		n.paintLive(dc, x, cw)
	case "games":
		n.paintGames(dc, x, cw)
	case "profiles":
		n.paintProfiles(dc, x, cw)
	case "benchmark":
		n.paintBenchmark(dc, x, cw)
	case "input":
		n.paintInput(dc, x, cw)
	case "keybinds":
		n.paintKeybinds(dc, x, cw)
	case "assistant":
		n.paintAssistant(dc, x, cw, answer)
	case "network":
		n.paintNetwork(dc, x, cw)
	case "repairs":
		n.paintRepairs(dc, x, cw)
	case "explain":
		n.paintExplain(dc, x, cw)
	case "developer":
		n.paintDeveloper(dc, x, cw)
	case "settings":
		n.paintSettings(dc, x, cw, identity)
	}
}

func valueAt(m map[string]interface{}, key string) string {
	v := m[key]
	if v == nil {
		return "Unknown"
	}
	return fmt.Sprint(v)
}
func (n *nativeApp) paintOverview(dc uintptr, x, cw int) {
	n.heading(dc, "SYSTEM DASHBOARD", "Automatic scan: see what Windows found before changing any settings.", x, 107, cw)
	n.app.mu.RLock()
	hw := n.app.hardware
	adv := append([]Advice(nil), n.app.advice...)
	gameCount := len(n.app.games)
	preset := n.app.config.Preset
	scanTime := n.app.scanned
	n.app.mu.RUnlock()
	cpu := asMaps(hw["CPU"])
	gs := asMaps(hw["GPU"])
	osys := asMaps(hw["OS"])
	cpuName, gpuName, osName := "CPU unavailable", "GPU unavailable", "Windows 10 / 11"
	if len(cpu) > 0 {
		cpuName = valueAt(cpu[0], "Name")
	}
	if len(gs) > 0 {
		gpuName = valueAt(gs[0], "Name")
	}
	if len(osys) > 0 {
		osName = valueAt(osys[0], "Caption")
	}
	gap := 12
	wid := (cw - 2*gap) / 3
	n.card(dc, x, 199, wid, 138, "PROCESSOR", short(cpuName, 42), "AMD Ryzen and other supported Windows CPUs")
	n.card(dc, x+wid+gap, 199, wid, 138, "GRAPHICS", short(gpuName, 42), "NVIDIA driver data when supported")
	n.card(dc, x+2*(wid+gap), 199, wid, 138, "DETECTED GAMES", fmt.Sprintf("%d installed", gameCount), "Steam, Epic, and common launcher locations")
	n.panel(dc, x, 354, cw, 150)
	n.text(dc, "SCAN RESULT", x+20, 367, cw-40, 33, red, n.fontBold, false)
	n.wrap(dc, fmt.Sprintf("OS: %s    •    Active preset: %s\nLast scan: %s\nWindows pointer speed: %v / 20    •    Power plan: %v", osName, preset, scanTime, hw["mouseSpeed"], hw["powerPlan"]), x+20, 400, cw-40, 100)
	n.button(dc, "rescan", "RE-SCAN HARDWARE + GAMES", x, 522, 274, 44, true)
	n.button(dc, "nav:profiles", "VIEW OPTIMIZATIONS", x+286, 522, 220, 44, false)
	n.text(dc, "DEVICE-SPECIFIC RECOMMENDATIONS", x, 586, cw, 34, red, n.fontBold, false)
	y := 622
	for i, a := range adv {
		if i >= 3 {
			break
		}
		if y > 735 {
			break
		}
		n.panel(dc, x, y, cw, 58)
		n.text(dc, a.Title, x+15, y+5, cw-30, 24, white, n.fontBold, false)
		n.text(dc, a.Body, x+15, y+29, cw-30, 25, muted, n.fontSmall, false)
		y += 67
	}
}
func (n *nativeApp) paintGames(dc uintptr, x, cw int) {
	n.heading(dc, "AUTO-DETECTED GAMES", "Select a game to load specific keybinds, mouse profiles and benchmarks.", x, 106, cw)
	n.app.mu.RLock()
	games := append([]Game(nil), n.app.games...)
	n.app.mu.RUnlock()
	n.panel(dc, x, 199, cw, 376)
	if len(games) == 0 {
		n.wrap(dc, "No supported game installations were found. Add an .exe path manually below. Some Game Pass/UWP installations are not automatically discoverable.", x+20, 225, cw-40, 90)
	}
	start := n.scroll
	if start >= len(games) {
		start = 0
	}
	end := start + 7
	if end > len(games) {
		end = len(games)
	}
	for i := start; i < end; i++ {
		y := 208 + (i-start)*49
		if i == n.selected {
			n.rect(dc, x+10, y, cw-20, 45, cardAccent)
			n.rect(dc, x+10, y, 4, 45, red)
		}
		n.text(dc, short(games[i].Name, 46), x+24, y+2, cw-320, 25, white, n.fontBold, false)
		n.text(dc, games[i].Platform+"  •  "+short(games[i].Source, 75), x+24, y+23, cw-65, 22, muted, n.fontSmall, false)
		n.hits = append(n.hits, hitBox{winRect{int32(x + 10), int32(y), int32(x + cw - 10), int32(y + 45)}, fmt.Sprintf("choose:%d", i)})
	}
	if len(games) > 0 {
		n.button(dc, "launch", "LAUNCH SELECTED GAME", x+15, 547, 274, 42, true)
		n.button(dc, "nav:keybinds", "KEYBINDS", x+301, 547, 158, 42, false)
		n.button(dc, "nav:input", "DPI GUIDE", x+469, 547, 148, 42, false)
	}
	n.small(dc, "ADD OR FIX A GAME (ENTER DISPLAY NAME AND FULL EXE PATH)", x+20, 580, cw-30)
	n.button(dc, "addgame", "ADD CUSTOM GAME", x+20, 652, 205, 42, false)
	n.button(dc, "setexe", "USE PATH FOR SELECTED", x+241, 652, 245, 42, false)
	n.text(dc, "Tip: scroll to browse more games. Games detected from launchers can require the launcher to start first.", x+20, 716, cw-40, 64, muted, n.fontSmall, true)
}
func (n *nativeApp) paintProfiles(dc uintptr, x, cw int) {
	n.heading(dc, "OPTIMIZATION PRESETS", "Strict latency mode blocks speculative Windows tweaks by default. Toggle in Settings to experiment.", x, 106, cw)
	for i, p := range []struct{ name, what, loss string }{
		{"Competitive", "Game Mode on; Game DVR background capture off.", "Game Bar instant replay stops until restored."},
		{"Balanced", "Game Mode on; keep background capture settings.", "Little/no change on a system already optimized."},
		{"Creator", "Game Mode on; leave recording and overlays available.", "Recording still uses CPU, GPU and disk bandwidth."},
	} {
		y := 211 + i*145
		n.panel(dc, x, y, cw, 129)
		n.text(dc, p.name+" MODE", x+19, y+10, cw-260, 32, red, n.fontBold, false)
		n.text(dc, "WHAT IT DOES: "+p.what, x+19, y+46, cw-260, 36, white, n.font, false)
		n.text(dc, "TRADEOFF: "+p.loss, x+19, y+88, cw-260, 27, muted, n.fontSmall, false)
		if n.app.preferencesSnapshot().LatencySafe {
			n.button(dc, "nav:settings", "SAFE MODE", x+cw-175, y+45, 154, 42, false)
		} else {
			n.button(dc, "preset:"+p.name, "APPLY", x+cw-175, y+45, 154, 42, true)
		}
	}
	n.button(dc, "restore", "RESTORE ORIGINAL SETTINGS", x, 659, 320, 45, false)
	n.wrap(dc, "Keep Defender enabled. Do not blindly disable Windows services, core isolation or drivers. Test actual frame times before deciding whether a preset helped.", x+340, 666, cw-352, 66)
}
func (n *nativeApp) paintBenchmark(dc uintptr, x, cw int) {
	n.heading(dc, "FRAME TIME LAB", "Compare real 1% lows and frame pacing, not only the FPS counter.", x, 106, cw)
	n.app.mu.RLock()
	present := findPresentMon(n.app.config.PresentMonPath)
	hist := append([]Benchmark(nil), n.app.history...)
	capturing := n.app.capturing
	n.app.mu.RUnlock()
	n.panel(dc, x, 198, cw, 168)
	status := "NOT FOUND — download PresentMon console"
	if present != "" {
		status = "READY: " + short(present, 85)
	}
	n.text(dc, "PRESENTMON: "+status, x+18, 214, cw-30, 36, func() uintptr {
		if present != "" {
			return green
		}
		return red
	}(), n.fontBold, false)
	n.wrap(dc, "Enter the full path to PresentMon.exe. Open the selected game before capture. The engine requests Windows frame presentation data and does not inject code into the game.", x+18, 249, cw-36, 43)
	n.button(dc, "savepresentmon", "SAVE PATH", x+18, 336, 184, 42, false)
	n.button(dc, "tool:presentmon", "OFFICIAL DOWNLOAD", x+216, 336, 234, 42, false)
	n.button(dc, "benchmark", "CAPTURE 30 SEC", x+465, 336, 215, 42, true)
	txt := "No capture running"
	if capturing {
		txt = "Capture in progress — keep the game open"
	}
	n.text(dc, txt, x, 391, cw, 30, muted, n.font, false)
	bw := (cw - 24) / 3
	n.button(dc, "latencybaseline", "CAPTURE BASELINE", x, 425, bw, 39, false)
	n.button(dc, "latencyafter", "CAPTURE AFTER", x+bw+12, 425, bw, 39, false)
	n.button(dc, "latencycompare", "COMPARE LATENCY", x+2*(bw+12), 425, bw, 39, true)
	n.text(dc, "RECENT BENCHMARK RESULTS", x, 477, cw, 35, red, n.fontBold, false)
	if len(hist) == 0 {
		n.wrap(dc, "No recordings yet. Capture the same scene before and after a change. 1% low FPS describes how slow the slowest 1% of frames are; higher is generally smoother.", x, 519, cw, 105)
	}
	for i := len(hist) - 1; i >= 0 && i >= len(hist)-4; i-- {
		j := len(hist) - 1 - i
		y := 520 + j*71
		if y > 757 {
			break
		}
		b := hist[i]
		n.panel(dc, x, y, cw, 61)
		n.text(dc, short(b.Game+"  /  "+b.Label, 65), x+14, y+6, cw-300, 27, white, n.fontBold, false)
		n.text(dc, fmt.Sprintf("AVG %.1f FPS   |   1%% LOW %.1f   |   0.1%% LOW %.1f   |   P99 %.1f ms", b.AvgFPS, b.Low1, b.Low01, b.P99ms), x+14, y+32, cw-25, 22, muted, n.fontSmall, false)
	}
}
func (n *nativeApp) paintInput(dc uintptr, x, cw int) {
	n.heading(dc, "MOUSE DPI + SENSITIVITY", "Measured mouse hardware DPI is not available through standard Windows interfaces.", x, 106, cw)
	g, ok := n.game()
	gameName := "Select a game in GAME LIBRARY"
	if ok {
		gameName = g.Name
	}
	guide := chooseGuide(gameName)
	n.panel(dc, x, 200, cw, 190)
	n.text(dc, "SELECTED GAME: "+gameName, x+18, 211, cw-36, 38, red, n.fontBold, false)
	n.wrap(dc, "DPI is set inside your physical mouse or its manufacturer app (Logitech G Hub, Razer Synapse, etc.). Windows pointer speed is NOT the same as sensor DPI. Enter what your mouse software reports.", x+18, 255, cw-36, 50)
	n.small(dc, "MOUSE DPI", x+20, 291, 190)
	n.small(dc, "IN-GAME SENSITIVITY", x+210, 291, 195)
	n.small(dc, "POLLING RATE HZ", x+402, 291, 185)
	n.button(dc, "saveinput", "SAVE THIS GAME PROFILE", x+20, 361, 267, 42, true)
	n.button(dc, "tool:mouse", "OPEN MOUSE SETTINGS", x+303, 361, 265, 42, false)
	n.text(dc, "GAME-SPECIFIC SENSITIVITY GUIDE", x, 442, cw, 39, red, n.fontBold, false)
	n.panel(dc, x, 491, cw, 208)
	n.wrap(dc, guide.SensNote, x+18, 503, cw-36, 67)
	n.wrap(dc, "Recommended setups are starting points, not measurements from your exact mouse or official universal pro settings.", x+18, 582, cw-36, 47)
	y := 648
	for _, s := range guide.Setups {
		if y > 752 {
			break
		}
		n.text(dc, fmt.Sprintf("%s:  %d DPI / %.3f in-game   — %s", s.Title, s.DPI, s.Sens, s.Description), x+19, y, cw-36, 30, white, n.fontSmall, false)
		y += 32
	}
}
func (n *nativeApp) paintKeybinds(dc uintptr, x, cw int) {
	n.heading(dc, "KEYBINDS + CONTROLLER", "Popular, practical starting points for games detected on this PC.", x, 106, cw)
	g, ok := n.game()
	gameName := "General FPS"
	if ok {
		gameName = g.Name
	}
	guide := chooseGuide(gameName)
	n.panel(dc, x, 199, cw, 110)
	n.text(dc, guide.Title+"  /  "+guide.Genre, x+18, 212, cw-36, 35, red, n.fontBold, false)
	n.wrap(dc, guide.GameNotes, x+18, 252, cw-36, 53)
	n.text(dc, "ACTION                           SUGGESTED                   ALTERNATE", x+8, 334, cw-15, 36, red, n.fontSmall, false)
	top := 365
	for i, b := range guide.Binds {
		if i >= 7 {
			break
		}
		y := top + i*47
		n.panel(dc, x, y, cw, 42)
		n.text(dc, short(b.Action, 27), x+12, y+4, cw/3-16, 28, white, n.fontSmall, false)
		n.text(dc, short(b.Key, 30), x+cw/3, y+4, cw/3, 28, red, n.fontSmall, false)
		n.text(dc, short(b.Alternative, 27), x+2*cw/3, y+4, cw/3-15, 28, muted, n.fontSmall, false)
	}
	n.text(dc, "VIDEO: "+guide.VideoNotes, x+16, 693, cw-35, 43, white, n.fontSmall, true)
	n.text(dc, "CONTROLLER: "+guide.ControllerNotes, x+16, 738, cw-35, 50, muted, n.fontSmall, true)
}
func (n *nativeApp) paintAssistant(dc uintptr, x, cw int, answer string) {
	n.heading(dc, "TROUBLESHOOTING ASSISTANT", "Offline advice works instantly. Online AI requires your own API key and consent.", x, 106, cw)
	n.panel(dc, x, 199, cw, 149)
	n.text(dc, "ASK A QUESTION IN PLAIN ENGLISH", x+18, 208, cw-36, 30, red, n.fontBold, false)
	n.button(dc, "askoffline", "ASK OFFLINE ASSISTANT", x+18, 304, 251, 42, true)
	n.button(dc, "askonline", "ASK ONLINE AI", x+283, 304, 198, 42, false)
	n.panel(dc, x, 357, cw, 133)
	n.wrap(dc, "Optional OpenAI API key (password field below). This key is NOT saved. The online service receives your question, but no hardware information unless you explicitly opt in.", x+18, 365, cw-36, 53)
	n.text(dc, "AI ANSWER / DIAGNOSTIC NEXT STEPS", x, 498, cw, 37, red, n.fontBold, false)
	n.panel(dc, x, 540, cw, 204)
	if answer == "" {
		answer = "Try: Why is my mouse lagging? / Why do I get low 1% lows? / Why does my GPU hit 100%? / How can I fix Siege stutters?\n\nThe offline assistant uses tested troubleshooting rules, not a live language model."
	}
	n.wrap(dc, answer, x+18, 551, cw-36, 179)
}
func (n *nativeApp) paintNetwork(dc uintptr, x, cw int) {
	n.heading(dc, "NETWORK MANAGER", "Diagnose delays without changing undocumented network registry keys.", x, 106, cw)
	n.panel(dc, x, 199, cw, 220)
	n.text(dc, "PING TARGET (HOSTNAME OR IP ADDRESS)", x+18, 216, cw-36, 34, red, n.fontBold, false)
	n.button(dc, "ping", "RUN 4-PACKET PING", x+cw-195, 271, 175, 42, true)
	n.wrap(dc, "Ping measures round-trip delay to a host. Lower ping can help online responsiveness, but network tweaking cannot force a better route to a game server. Wi-Fi interference and background downloads often matter more.", x+18, 332, cw-36, 80)
	n.button(dc, "repair:ipconfig", "NETWORK DETAILS", x, 444, 239, 44, false)
	n.button(dc, "repair:netadapters", "NETWORK ADAPTERS", x+250, 444, 229, 44, false)
	n.button(dc, "repair:dns", "FLUSH DNS CACHE", x+491, 444, 218, 44, false)
	n.text(dc, "LAST JOBS", x, 525, cw, 38, red, n.fontBold, false)
	n.paintJobs(dc, x, 568, cw, 3)
}
func (n *nativeApp) paintRepairs(dc uintptr, x, cw int) {
	n.heading(dc, "WINDOWS REPAIR LAB", "Every command has a plain-language explanation and asks permission first.", x, 106, cw)
	items := []struct{ action, title, desc string }{
		{"sfc", "SYSTEM FILE CHECKER", "Scans Windows files; restores damaged protected files."},
		{"dismcheck", "CHECK WINDOWS IMAGE", "Checks Windows servicing files without applying repairs."},
		{"dism", "REPAIR WINDOWS IMAGE", "Attempts to restore damaged Windows component storage."},
		{"disk", "SCAN DISK FILE SYSTEM", "Checks drive file system for errors without offline repair."},
		{"events", "CRASH / GPU EVENTS", "Read-only search for important Windows crash events."},
		{"processes", "MEMORY-HUNGRY APPS", "Read-only list of processes using the most RAM."},
		{"drivers", "INSTALLED DRIVERS", "Read-only driver inventory for troubleshooting."},
	}
	for i, item := range items {
		col := i % 2
		row := i / 2
		bw := (cw - 12) / 2
		y := 205 + row*109
		xx := x + col*(bw+12)
		n.panel(dc, xx, y, bw, 99)
		n.text(dc, item.title, xx+12, y+7, bw-155, 29, red, n.fontBold, false)
		n.text(dc, item.desc, xx+12, y+45, bw-28, 50, white, n.fontSmall, true)
		n.button(dc, "repair:"+item.action, "RUN", xx+bw-99, y+10, 85, 38, false)
	}
	n.text(dc, "JOB HISTORY — automatic repairs may require administrator rights", x, 662, cw, 33, red, n.fontBold, false)
	n.paintJobs(dc, x, 703, cw, 1)
}
func (n *nativeApp) paintJobs(dc uintptr, x, y, cw, count int) {
	n.app.mu.RLock()
	jobs := append([]Job(nil), n.app.jobs...)
	n.app.mu.RUnlock()
	if len(jobs) == 0 {
		n.wrap(dc, "No diagnostic jobs yet.", x+15, y, cw-30, 30)
		return
	}
	for i := 0; i < len(jobs) && i < count; i++ {
		j := jobs[i]
		yy := y + i*64
		n.panel(dc, x, yy, cw, 58)
		n.text(dc, j.Kind+" • "+j.Status, x+12, yy+3, cw-25, 24, red, n.fontBold, false)
		out := strings.TrimSpace(j.Output)
		if out == "" {
			out = "Job started; check again for the result."
		}
		n.text(dc, short(strings.ReplaceAll(out, "\n", "  "), 180), x+12, yy+30, cw-25, 26, muted, n.fontSmall, false)
	}
}
func (n *nativeApp) paintDeveloper(dc uintptr, x, cw int) {
	n.heading(dc, "DEVELOPER CONSOLE", "Protected diagnostics; developer mode does not grant Windows administrator rights.", x, 106, cw)
	n.panel(dc, x, 199, cw, 201)
	n.text(dc, "DEVELOPER ACCESS", x+18, 211, cw-36, 34, red, n.fontBold, false)
	n.wrap(dc, "Set a strong passphrase (12+ characters) once. To reopen this console later, unlock with that passphrase. Only diagnostic exports are exposed; developer mode cannot bypass antivirus or anti-cheat.", x+18, 245, cw-36, 48)
	n.button(dc, "devsetup", "CREATE PASSPHRASE", x+20, 357, 222, 42, false)
	n.button(dc, "devunlock", "UNLOCK DEVELOPER", x+256, 357, 220, 42, true)
	n.button(dc, "devlock", "LOCK", x+490, 357, 98, 42, false)
	n.mu.Lock()
	dev := n.developer
	n.mu.Unlock()
	if dev {
		n.panel(dc, x, 440, cw, 190)
		n.text(dc, "UNLOCKED FOR THIS SESSION", x+18, 454, cw-34, 39, green, n.fontBold, false)
		n.wrap(dc, "You can export a read-only system report for debugging. Never share the report publicly without checking for computer names, file paths or other private details.", x+18, 508, cw-36, 68)
		n.button(dc, "devreport", "EXPORT DIAGNOSTICS JSON", x+18, 582, 291, 42, true)
	} else {
		n.wrap(dc, "LOCKED. Sign in as a normal user first, then enter your own developer passphrase. A Google account is not automatically a developer account.", x+18, 453, cw-36, 124)
	}
}
func (n *nativeApp) paintPlus(dc uintptr, x, cw int) {
	n.heading(dc, "PLUS MEMBERSHIP", "Basic is free. Plus is $1.99/month, managed securely through Stripe.", x, 106, cw)
	n.mu.Lock()
	p := n.plus
	n.mu.Unlock()
	plan := "BASIC / FREE"
	if p.Plan == "plus" {
		if p.Source == "complimentary" {
			plan = "PRO / COMPLIMENTARY GIFT"
		} else {
			plan = "PLUS / SUBSCRIBER"
		}
	}
	n.panel(dc, x, 196, cw, 84)
	n.text(dc, "SUBSCRIPTION STATUS", x+18, 204, cw-40, 27, red, n.fontBold, false)
	n.text(dc, plan+"    "+short(p.Message, 67), x+18, 235, cw-40, 32, white, n.fontSmall, false)
	n.small(dc, "SECURE PLUS SERVER (HTTPS address supplied by the publisher)", x+18, 284, cw-40)
	n.text(dc, "ENTER YOUR EMAIL FOR STRIPE CHECKOUT", x+18, 356, cw-40, 28, red, n.fontBold, false)
	n.wrap(dc, "Plus unlocks extended frame-analysis guidance, advanced game profiles and future features. You always retain Basic tools.", x+18, 439, cw-40, 60)
	n.button(dc, "pluscheckout", "SUBSCRIBE — $1.99 / MONTH", x+18, 505, 278, 44, true)
	n.button(dc, "pluspoll", "CHECK PURCHASE STATUS", x+313, 505, 263, 44, false)
	n.small(dc, "ALREADY HAVE A PLUS KEY? ENTER IT BELOW", x+18, 557, cw-40)
	n.button(dc, "plusactivate", "ACTIVATE KEY", x+18, 639, 185, 44, true)
	n.button(dc, "plusverify", "VERIFY MEMBERSHIP", x+218, 639, 219, 44, false)
	n.button(dc, "plusmanage", "MANAGE OR CANCEL", x+452, 639, 192, 44, false)
	n.wrap(dc, "Use SUBSCRIBE for $1.99/month or ACTIVATE KEY for a developer-issued complimentary Pro pass. Gift passes have no charge and expire on their issued date. Both types are verified by the server and tied to one installation. Only subscribers have Stripe billing to manage.", x+18, 701, cw-40, 95)
}
func (n *nativeApp) paintSettings(dc uintptr, x, cw int, identity string) {
	n.heading(dc, "APPLICATION SETTINGS", "Manage gaming performance safety, monitoring overhead and device tools.", x, 106, cw)
	game, p := n.app.guardSnapshot()
	n.panel(dc, x, 195, cw, 312)
	n.text(dc, "AUTOMATIC GAME GUARD", x+18, 204, cw-36, 33, red, n.fontBold, false)
	if game != "" {
		n.text(dc, "DETECTED: "+short(game, 50), x+18, 233, cw-36, 25, green, n.fontSmall, false)
	} else {
		n.text(dc, "No supported game currently detected", x+18, 233, cw-36, 25, muted, n.fontSmall, false)
	}
	onOff := func(enabled bool) string {
		if enabled {
			return "ON"
		}
		return "OFF"
	}
	n.button(dc, "pref:detect", "GAME DETECTION: "+onOff(p.DetectGames), x+18, 270, 285, 44, false)
	n.wrap(dc, "Check running games about every 15 seconds. No game injection or administrator permission.", x+326, 272, cw-345, 52)
	n.button(dc, "pref:quiet", "LOW-USAGE MODE: "+onOff(p.QuietWhileGaming), x+18, 331, 285, 44, true)
	n.wrap(dc, "While gaming, Sawcha reduces its own live monitoring and redraw frequency.", x+326, 334, cw-345, 49)
	n.button(dc, "pref:warn", "CHANGE WARNINGS: "+onOff(p.WarnBeforeChanges), x+18, 391, 285, 44, false)
	n.wrap(dc, "Warn before system changes while a detected game is running. Enabled by default.", x+326, 393, cw-345, 48)
	n.button(dc, "pref:latency", "STRICT LATENCY: "+onOff(p.LatencySafe), x+18, 450, 285, 43, p.LatencySafe)
	n.button(dc, "pref:defaults", "RESET DEFAULTS", x+326, 450, 225, 43, false)
	n.text(dc, "DEVICE CONTROL PANELS", x, 524, cw, 35, red, n.fontBold, false)
	tools := []struct{ key, title string }{{"nvidia", "NVIDIA"}, {"amd", "AMD"}, {"mouse", "MOUSE"}, {"controller", "CONTROLLER"}, {"keyboard", "KEYBOARD"}, {"graphics", "GRAPHICS"}, {"drivers", "DRIVERS"}, {"device", "DEVICE MANAGER"}, {"startup", "STARTUP APPS"}}
	for i, t := range tools {
		col, row := i%3, i/3
		bw := (cw - 26) / 3
		xx := x + col*(bw+13)
		yy := 566 + row*52
		n.button(dc, "tool:"+t.key, t.title, xx, yy, bw, 39, false)
	}
	if identity == "" {
		identity = "Local user"
	}
	n.small(dc, "ACCOUNT: "+short(identity, 51), x, 734, cw-350)
	n.button(dc, "verify", "VERIFY APP SIGNATURE", x, 765, 220, 39, false)
	n.button(dc, "logout", "SIGN OUT", x+240, 765, 143, 39, false)
	n.button(dc, "signing", "SIGNING HELP", x+400, 765, 170, 39, false)
}

func runNative(a *App) {
	jar, _ := cookiejar.New(nil)
	n := &nativeApp{app: a, controls: map[string]uintptr{}, plus: loadPlus(), page: "overview", selected: 0, status: "Starting system scanner...", client: &http.Client{Jar: jar, Timeout: 50 * time.Second}}
	native = n
	inst, _, _ := getModuleHandle.Call(0)
	cur, _, _ := loadCursor.Call(0, 32512)
	class := "SAWCHA_NativeOptimizer_Window_v5"
	wc := winClass{Size: uint32(unsafe.Sizeof(winClass{})), Style: 3, WndProc: syscall.NewCallback(nativeWndProc), Instance: inst, Cursor: cur, ClassName: wide(class)}
	registerClass.Call(uintptr(unsafe.Pointer(&wc)))
	hwnd, _, err := createWindow.Call(0, uintptr(unsafe.Pointer(wide(class))), uintptr(unsafe.Pointer(wide("SAWCHA // OPTIMIZER — Built by sawcha | Native Windows Edition"))), wsOverlappedWindow|wsVisible|wsClipChildren, 90, 65, 1350, 880, 0, 0, inst, 0)
	if hwnd == 0 {
		fmt.Fprintln(os.Stderr, "Unable to create Windows window:", err)
		return
	}
	n.hwnd = hwnd
	go func() {
		n.mu.Lock()
		p := n.plus
		n.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = plusVerify(ctx, &p)
		n.mu.Lock()
		n.plus = p
		n.mu.Unlock()
		n.invalidate()
	}()
	showWindow.Call(hwnd, swShow)
	updateWindow.Call(hwnd)
	var msg winMsg
	for {
		got, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(got) <= 0 {
			break
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// DPI calculations are explicitly game-specific. Generic eDPI values do not transfer
// between different game sensitivity scales.
func edpi(dpi int, sens float64, unit string) float64 {
	if unit == "percent" {
		return float64(dpi) * sens / 100
	}
	if unit == "none" {
		return math.NaN()
	}
	return float64(dpi) * sens
}

var _ = errors.New

func (n *nativeApp) paintExplain(dc uintptr, x, cw int) {
	n.heading(dc, "WHAT EVERY SETTING DOES", "Plain English: how it works, when it helps, drawbacks and how to undo it.", x, 106, cw)
	// Scroll the library using the mouse wheel, without changing a single system setting.
	start := n.scroll / 2
	if start >= len(explainers) {
		start = len(explainers) - 1
	}
	if start < 0 {
		start = 0
	}
	y := 199
	for i := start; i < len(explainers) && i < start+3; i++ {
		e := explainers[i]
		n.panel(dc, x, y, cw, 173)
		n.text(dc, e.Title+"  /  "+e.Category, x+17, y+7, cw-32, 31, red, n.fontBold, false)
		n.text(dc, "WHAT: "+e.Summary, x+18, y+43, cw-36, 32, white, n.fontSmall, true)
		n.text(dc, "HOW: "+e.How, x+18, y+75, cw-36, 33, white, n.fontSmall, true)
		n.text(dc, "HELPS: "+e.Helps, x+18, y+108, cw-36, 29, green, n.fontSmall, true)
		n.text(dc, "CAUTION: "+e.Risks, x+18, y+135, cw-36, 37, muted, n.fontSmall, true)
		y += 185
	}
	n.text(dc, fmt.Sprintf("TOPIC %d OF %d  •  SCROLL TO READ MORE  •  NO CHANGES ARE MADE HERE", start+1, len(explainers)), x, 765, cw, 31, muted, n.fontSmall, false)
}

func (n *nativeApp) paintLive(dc uintptr, x, cw int) {
	n.app.mu.Lock()
	n.app.lastMonitor = time.Now()
	live := n.app.live
	n.app.mu.Unlock()
	n.heading(dc, "LIVE PERFORMANCE MONITOR", "Samples Windows CPU, RAM, NVIDIA GPU and the largest background programs.", x, 106, cw)
	c := asMaps(live["CPU"])
	o := asMaps(live["OS"])
	nv := asMaps(live["nvidia"])
	cpu := "Scanning..."
	ram := "Scanning..."
	gpu := "NVIDIA sensor unavailable"
	if len(c) > 0 {
		cpu = fmt.Sprintf("%.0f%%", toFloat(c[0]["LoadPercentage"]))
	}
	if len(o) > 0 {
		total := toFloat(o[0]["TotalVisibleMemorySize"])
		free := toFloat(o[0]["FreePhysicalMemory"])
		if total > 0 {
			ram = fmt.Sprintf("%.1f / %.1f GB", (total-free)/1048576, total/1048576)
		}
	}
	if len(nv) > 0 {
		gpu = fmt.Sprintf("%.0f%%  /  %.0f°C", toFloat(nv[0]["usage"]), toFloat(nv[0]["tempC"]))
	}
	gap := 12
	wide := (cw - 2*gap) / 3
	n.card(dc, x, 202, wide, 137, "CPU UTILIZATION", cpu, "High CPU load during gameplay can limit FPS")
	n.card(dc, x+wide+gap, 202, wide, 137, "MEMORY USED", ram, "Heavy RAM use can contribute to stutters")
	n.card(dc, x+2*(wide+gap), 202, wide, 137, "NVIDIA GPU / TEMPERATURE", gpu, "Check GPU load and temperatures")
	n.text(dc, "HEAVIEST RUNNING PROCESSES", x, 369, cw, 44, red, n.fontBold, false)
	n.panel(dc, x, 425, cw, 296)
	items := asMaps(live["Processes"])
	if len(items) == 0 {
		n.wrap(dc, "Waiting for Windows to provide process data. The first sample can take several seconds. This display only reads information, and never shuts down programs.", x+16, 440, cw-32, 76)
	}
	for i, p := range items {
		if i >= 7 {
			break
		}
		yy := 432 + i*39
		n.text(dc, fmt.Sprintf("%d. %s", i+1, short(valueAt(p, "ProcessName"), 38)), x+16, yy, cw-260, 28, white, n.font, false)
		n.text(dc, fmt.Sprintf("%.0f MB RAM", toFloat(p["WorkingSet64"])/1048576), x+cw-222, yy, 205, 28, red, n.fontSmall, false)
	}
	n.text(dc, "Information refreshes roughly every 5 seconds while this page is visible. A high process memory value alone is not proof of a problem.", x+14, 733, cw-30, 56, muted, n.fontSmall, true)
}
