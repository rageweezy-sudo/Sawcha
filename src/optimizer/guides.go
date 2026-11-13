package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Curated recommendations, not changes to game configuration files. Ranges are
// suggested starting points, not live readings of players' settings.
type Bind struct {
	Action      string `json:"action"`
	Key         string `json:"key"`
	Alternative string `json:"alternative"`
	Why         string `json:"why"`
}
type Setup struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	DPI         int     `json:"dpi"`
	Sens        float64 `json:"sens"`
	Style       string  `json:"style"`
}
type GameGuide struct {
	ID              string        `json:"id"`
	Title           string        `json:"title"`
	Aliases         []string      `json:"aliases,omitempty"`
	Genre           string        `json:"genre"`
	SensUnit        string        `json:"sensUnit"`
	SensMin         float64       `json:"sensMin"`
	SensMax         float64       `json:"sensMax"`
	SensNote        string        `json:"sensNote"`
	GameNotes       string        `json:"gameNotes"`
	VideoNotes      string        `json:"videoNotes"`
	ControllerNotes string        `json:"controllerNotes"`
	Setups          []Setup       `json:"setups"`
	Binds           []Bind        `json:"binds"`
	Sources         []GuideSource `json:"sources,omitempty"`
}
type GuideSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}
type Explainer struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Title    string `json:"title"`
	Summary  string `json:"summary"`
	How      string `json:"how"`
	Helps    string `json:"helps"`
	Risks    string `json:"risks"`
	Undo     string `json:"undo"`
}
type SensitivitySetting struct {
	DPI       int     `json:"dpi"`
	Sens      float64 `json:"sens"`
	PollingHz int     `json:"pollingHz"`
}

func binds(rows ...[4]string) []Bind {
	out := make([]Bind, 0, len(rows))
	for _, x := range rows {
		out = append(out, Bind{x[0], x[1], x[2], x[3]})
	}
	return out
}
func setup(title, why string, dpi int, sens float64, style string) Setup {
	return Setup{title, why, dpi, sens, style}
}

var siegeBinds = binds(
	[4]string{"Move", "W / A / S / D", "Keep standard", "Keeping movement around your left hand makes every other key easier to reach."},
	[4]string{"Lean left / right", "Q / E", "Mouse side buttons if comfortable", "Q/E is easy to learn; mouse buttons can free your movement fingers, but may shake your aim."},
	[4]string{"Crouch", "C", "Left Ctrl", "Choose whichever you can press without releasing movement keys."},
	[4]string{"Prone", "Left Ctrl", "Z", "Keep a separate, deliberate button to avoid going prone accidentally."},
	[4]string{"Reload", "R", "Standard", "A reliable reload key is more useful than a complicated macro."},
	[4]string{"Interact", "F", "E, if lean moved to mouse", "Make defusing, barricades and pickups easy to reach."},
	[4]string{"Melee", "V", "Mouse button", "A reachable melee key helps without requiring a grip change."},
	[4]string{"Push-to-talk", "Mouse button / V", "Your preference", "Keeps communication available without accidentally opening your mic."},
	[4]string{"Primary gadget", "Middle mouse / G", "Mouse side button", "Pick one you can hit consistently while aiming."},
	[4]string{"Secondary gadget", "G / another reachable key", "Your preference", "Avoid accidental gadget use during tense gunfights."},
)
var typicalFPSBinds = binds(
	[4]string{"Move", "W / A / S / D", "Standard", "A familiar layout leaves your fingers free for precision."},
	[4]string{"Jump", "Space", "Mouse wheel where supported", "Space is reliable; some movement-heavy games benefit from wheel jump."},
	[4]string{"Crouch / slide", "Left Ctrl", "C", "Choose the easier key while moving diagonally."},
	[4]string{"Interact / use", "E / F", "Your preference", "Keep it near movement keys."},
	[4]string{"Reload", "R", "Standard", "Simple muscle memory avoids missed reloads."},
	[4]string{"Push-to-talk", "V / Mouse button", "Your preference", "Fast team communication without losing movement."},
	[4]string{"Primary / secondary weapon", "1 / 2", "Mouse wheel", "Direct number keys can reduce accidental wrong-weapon swaps."},
)
var gameGuides = []GameGuide{
	{ID: "siege", Title: "Rainbow Six Siege / Siege X", Aliases: []string{"rainbow six", "rainbowsix", "siege x"}, Genre: "Tactical FPS", SensUnit: "raw", SensMin: 3, SensMax: 12,
		SensNote:        "eDPI = mouse DPI × in-game hipfire sensitivity for the standard 0.02 multiplier. A custom multiplier changes comparisons. ADS sensitivity is separate.",
		GameNotes:       "Start with 400 or 800 DPI, then tune horizontal/vertical hipfire sensitivity. Set ADS for each optic separately. Lean keys should stay reachable while strafing. Use actual game input settings; this app never writes bindings into the game.",
		VideoNotes:      "Use fullscreen or the game's best exclusive-equivalent mode, native monitor refresh, and low-cost shadows/effects first. Test NVIDIA Reflex if available; avoid stacking several latency limiters. Keep render scale clear enough to spot operators.",
		ControllerNotes: "Use the smallest right-stick deadzone that does not drift; begin around 5–10% if the game exposes percentages and raise only to stop drift. Practice consistent ADS settings. Do not use unauthorized adapters or macros.",
		Setups:          []Setup{setup("Control-focused", "Common tactical starting feel; increase if 180° turns are too slow.", 400, 10, "Precision"), setup("Balanced 800 DPI", "Comparable hipfire eDPI to 400 × 10 using the standard multiplier.", 800, 5, "Balanced"), setup("Faster clearances", "A faster starting feel for close-range room clearing.", 800, 7, "Fast")}, Binds: siegeBinds,
		Sources: []GuideSource{{"R6 pro settings reference", "https://prosettings.net/lists/rainbow-six-siege/"}, {"R6 sensitivity discussion", "https://senslab.pro/pro-settings/r6siege"}}},
	{ID: "valorant", Title: "VALORANT", Aliases: []string{"valorant"}, Genre: "Tactical FPS", SensUnit: "raw", SensMin: 0.2, SensMax: 0.4,
		SensNote:  "eDPI = mouse DPI × VALORANT in-game sensitivity. Most players start near 200–320 eDPI; it cannot be compared directly to Siege eDPI.",
		GameNotes: "Keep walk, crouch, abilities and plant/defuse easy to reach without lifting movement fingers. Set mouse acceleration off in your mouse software if offered; VALORANT uses raw input.", VideoNotes: "Multithreaded rendering on where supported. Test Fullscreen, low/medium effects, VSync off and NVIDIA Reflex on if offered. Cap FPS sensibly if the frame-rate swings hard.", ControllerNotes: "VALORANT PC focuses on mouse and keyboard; controller support and matchmaking differ by platform. Use official in-game options only.", Setups: []Setup{setup("Precision", "Low sensitivity starting point.", 800, 0.27, "Precision"), setup("Balanced", "Near community median eDPI.", 800, 0.32, "Balanced"), setup("Fast flicks", "Higher starting range; may reduce micro precision.", 800, 0.4, "Fast")}, Binds: append(append([]Bind{}, typicalFPSBinds...), binds([4]string{"Walk", "Left Shift", "Standard", "Walk without giving up WASD movement."}, [4]string{"Ability 1 / 2 / 3", "C / Q / E", "Mouse side button for one ability", "Keep utility within reach."}, [4]string{"Ultimate", "X", "Standard", "Hard enough to press intentionally."})...), Sources: []GuideSource{{"VALORANT sensitivity distribution", "https://senscurve.com/valorant-pro-sensitivity/"}}},
	{ID: "cs2", Title: "Counter-Strike 2", Aliases: []string{"counter-strike 2", "counter strike 2", "cs2", "counterstrike"}, Genre: "Tactical FPS", SensUnit: "raw", SensMin: 0.6, SensMax: 1.6,
		SensNote: "eDPI = DPI × CS2 in-game sensitivity. Compare only with other CS2 setups.", GameNotes: "Keep walk on Shift, crouch on Ctrl, use on E and voice chat available. Some players use wheel-down to jump, but keep Space as a backup. Avoid jump/aim macros.", VideoNotes: "Prioritize stable 1% lows, full refresh rate and enough visibility. Benchmark Reflex, VSync and frame cap changes separately.", ControllerNotes: "Mouse and keyboard are the common competitive starting point. Controller aiming needs practice and game support.", Setups: []Setup{setup("Precision", "Controlled rifling baseline.", 800, 0.9, "Precision"), setup("Balanced", "Popular middle-range starting point.", 800, 1.1, "Balanced"), setup("Faster tracking", "A quicker but still measured starting point.", 800, 1.4, "Fast")}, Binds: append(append([]Bind{}, typicalFPSBinds...), binds([4]string{"Jump alternative", "Wheel down", "Space as backup", "Can make repeatable movement inputs easier; optional."}, [4]string{"Walk", "Left Shift", "Standard", "Keeps movement deliberate."}, [4]string{"Utility slots", "Dedicated keys", "C / X / Z as preferred", "Direct binds can save time versus cycling grenades."})...)},
	{ID: "fortnite", Title: "Fortnite", Aliases: []string{"fortnite"}, Genre: "Battle Royale / Builder", SensUnit: "percent", SensMin: 4, SensMax: 10,
		SensNote: "Fortnite uses percent-style mouse sensitivity. This app computes eDPI = DPI × (percentage / 100), e.g. 800 × 6% = 48. This is not directly comparable to VALORANT eDPI.", GameNotes: "For building, distribute wall, ramp, floor and cone across comfortable keys and mouse side buttons, but avoid overload. For Zero Build, prioritize sprint, slide, edit-free weapon swapping.", VideoNotes: "Performance Mode can improve FPS on modest hardware; test it against DX12 because some hardware behaves differently. Reduce shadows and effects before making text/players unreadable.", ControllerNotes: "Start with small deadzones that avoid drift, then tune edit/build multipliers gradually. Don't assume a pro's advanced curve works for everyone.", Setups: []Setup{setup("Aim control", "Slower turn speed for tracking.", 800, 5, "Precision"), setup("Balanced", "Solid starter for both build and Zero Build.", 800, 6.5, "Balanced"), setup("Fast building", "Faster turning baseline; separate ADS as needed.", 800, 8, "Fast")}, Binds: binds([4]string{"Movement", "W / A / S / D", "Standard", "The base for everything else."}, [4]string{"Wall", "Mouse side button", "Q", "Lets you place a wall while moving."}, [4]string{"Ramp", "Second mouse button", "E", "Quick reach for vertical protection."}, [4]string{"Floor", "C", "F", "A finger-friendly floor key."}, [4]string{"Cone", "Left Shift", "V", "Choose whichever is comfortable."}, [4]string{"Edit", "F", "E", "Easy reach matters more than copying a streamer."}, [4]string{"Reload / use", "R / E", "Change use if edit uses E", "Avoid double-assigned actions."}, [4]string{"Crouch / slide", "Left Ctrl", "C", "Keep available while moving."})},
	{ID: "apex", Title: "Apex Legends", Aliases: []string{"apex legends"}, Genre: "Battle Royale / Movement FPS", SensUnit: "raw", SensMin: 1.1, SensMax: 2.2,
		SensNote: "eDPI = DPI × Apex sensitivity; game-specific scale. ADS and per-optic settings can change the feel.", GameNotes: "Sprint, slide and jump should work while strafing. A mouse wheel jump option is popular with movement-focused players, but keep a stable primary bind.", VideoNotes: "Start low/medium settings on weaker PCs, disable heavy shadows and test stable FPS caps. Frame pacing is often more important than headline peak FPS.", ControllerNotes: "Begin at the lowest deadzone that prevents drift. Try default response curve before moving to advanced look controls. Confirm stick calibration.", Setups: []Setup{setup("Tracking", "Controlled close tracking.", 800, 1.2, "Precision"), setup("Balanced", "Moderate FPS baseline.", 800, 1.5, "Balanced"), setup("Movement", "Faster movement-oriented baseline.", 800, 1.9, "Fast")}, Binds: append(append([]Bind{}, typicalFPSBinds...), binds([4]string{"Tactical ability", "Q", "Mouse side button", "Keep accessible while tracking."}, [4]string{"Slide", "Left Ctrl", "C", "Avoid releasing movement keys."}, [4]string{"Ping", "Middle mouse", "Mouse button", "Quick nonverbal callouts."})...)},
	{ID: "cod", Title: "Call of Duty / Warzone", Aliases: []string{"call of duty", "warzone", "black ops", "modern warfare"}, Genre: "FPS / Battle Royale", SensUnit: "raw", SensMin: 2, SensMax: 8,
		SensNote: "Sensitivity scales and FOV change across COD titles/versions. The 800-DPI suggestions are illustrative starting points only; compare by 360-degree turn distance, not raw eDPI.", GameNotes: "Put sprint/tactical sprint, crouch/slide and plates where your movement hand can hit them. Use hold/toggle preferences deliberately.", VideoNotes: "A stable FPS cap, Reflex when supported, less costly shadows and suitable upscaling can help; compare visibility and frame-time graphs.", ControllerNotes: "Test dynamic versus standard response curve and low deadzone without drift. Aim-assist availability depends on game and platform.", Setups: []Setup{setup("Controlled", "Start low and increase until a 180° turn is comfortable.", 800, 3.5, "Precision"), setup("Moderate", "A reasonable sensitivity starting test.", 800, 5, "Balanced"), setup("Fast", "For fast turns if aiming stays consistent.", 800, 6.5, "Fast")}, Binds: append(append([]Bind{}, typicalFPSBinds...), binds([4]string{"Armor plates", "4 / mouse button", "Game dependent", "Keep plate use accessible under pressure."}, [4]string{"Tactical sprint", "Left Shift", "Auto sprint in game options", "Avoid unnecessary hand strain."})...)},
	{ID: "overwatch", Title: "Overwatch 2", Aliases: []string{"overwatch"}, Genre: "Hero Shooter", SensUnit: "raw", SensMin: 3, SensMax: 8,
		SensNote: "eDPI = DPI × Overwatch in-game sensitivity, not comparable to VALORANT or Siege.", GameNotes: "Keep ability 1 / 2 and ultimate easy to reach. Hero-specific settings can be important; support heroes may benefit from different sensitivity.", VideoNotes: "Favor high stable FPS and clear silhouettes. Use lower effects/shadows first and check render scale isn't unintentionally below 100%.", ControllerNotes: "Tune aim smoothing and deadzones gradually; these vary with controller and platform.", Setups: []Setup{setup("Precision", "Good baseline for hitscan tracking.", 800, 4, "Precision"), setup("Balanced", "A general-purpose starting feel.", 800, 5.5, "Balanced"), setup("Fast turns", "For high-mobility heroes.", 800, 7, "Fast")}, Binds: append(append([]Bind{}, typicalFPSBinds...), binds([4]string{"Ability 1 / 2", "Left Shift / E", "Mouse side buttons", "Ability access without compromising movement."}, [4]string{"Ultimate", "Q", "Standard", "Consistent activation across heroes."})...)},
	{ID: "rocket", Title: "Rocket League", Aliases: []string{"rocket league"}, Genre: "Sports / Driving", SensUnit: "none", SensNote: "Mouse DPI is not a meaningful performance setting for most Rocket League controller setups.", GameNotes: "Most competitive players use a controller. Prioritize separate boost, powerslide and air-roll binds that allow simultaneous presses.", VideoNotes: "Use stable FPS and the monitor's full refresh rate; disable visually expensive effects if the frame rate is unstable.", ControllerNotes: "A common starting point is boost on a bumper, powerslide + air roll on the opposite bumper, and jump on A / Cross. Learn one layout consistently rather than changing it daily.", Setups: []Setup{}, Binds: binds([4]string{"Jump", "A / Cross", "Default", "Reliable jump control."}, [4]string{"Boost", "RB / R1", "B / Circle", "Bumper boost can make boost + jump easier."}, [4]string{"Powerslide / air roll", "LB / L1", "X / Square", "Combined slide/roll is a popular layout."}, [4]string{"Air roll left", "X / Square", "Extra shoulder", "Optional; helps advanced aerial mechanics."}, [4]string{"Ball cam", "Y / Triangle", "Default", "Consistent view switching."}, [4]string{"Accelerate / reverse", "RT / R2 and LT / L2", "Standard", "Analog triggers allow smoother throttle input."})},
	{ID: "rust", Title: "Rust", Aliases: []string{"rust"}, Genre: "Survival FPS", SensUnit: "raw", SensMin: 0.4, SensMax: 1.2,
		SensNote: "Rust sensitivity scales differ from other shooters and may vary with configurations. Treat these values as feel tests, not universal pro numbers.", GameNotes: "Keep interact, crafting, freelook and voice chat close. Avoid binding shooting-related macros; play using permitted in-game settings.", VideoNotes: "Rust is often sensitive to RAM capacity, CPU load and texture/shadow settings. Try a stable graphics preset and measure in populated areas.", ControllerNotes: "Check in-game controller availability and mappings for your version; the keyboard/mouse setup is the default focus.", Setups: []Setup{setup("Precision", "A slower baseline for tracking.", 800, 0.5, "Precision"), setup("Balanced", "A moderate starting feel.", 800, 0.7, "Balanced"), setup("Fast", "Quicker look movement.", 800, 0.9, "Fast")}, Binds: append(append([]Bind{}, typicalFPSBinds...), binds([4]string{"Freelook", "Left Alt", "Preferred", "Look around without turning your character."}, [4]string{"Voice", "V", "Mouse button", "Easy team callouts."})...)},
	{ID: "minecraft", Title: "Minecraft", Aliases: []string{"minecraft", "java edition", "bedrock"}, Genre: "Sandbox / Creator", SensUnit: "none", SensNote: "Minecraft Java and Bedrock use different sensitivity scales; a single eDPI number is not reliable across editions.", GameNotes: "Put hotbar slots, sprint, sneak and inventory where comfortable. Avoid third-party input automation on multiplayer servers with anti-cheat rules.", VideoNotes: "Java performance depends on render distance, simulation distance, shaders and mods. Reduce render/simulation distance first; confirm mod compatibility. Bedrock has a different rendering engine.", ControllerNotes: "Use the edition's built-in controller settings where available; Java often requires an extra mapping tool.", Setups: []Setup{}, Binds: binds([4]string{"Movement", "W / A / S / D", "Standard", "Predictable movement."}, [4]string{"Jump / sneak", "Space / Left Shift", "Preferred", "Reliable building and movement."}, [4]string{"Sprint", "Left Ctrl", "Toggle sprint in settings", "Reduces finger fatigue."}, [4]string{"Inventory", "E", "Standard", "Close to movement."}, [4]string{"Hotbar slots", "1 through 9", "Mouse wheel", "Direct slots are faster when practiced."}, [4]string{"Drop", "Q", "Move if accidental drops", "Accidental drop can lose valuable items."})},
}

var genericGuide = GameGuide{
	ID: "general", Title: "General FPS / Unrecognized game", Genre: "General gaming", SensUnit: "none",
	SensNote:        "There is no universal mouse sensitivity across games. DPI is set in mouse hardware/software; tune the in-game slider by feel and measure turns on your mousepad.",
	GameNotes:       "Start with familiar movement and jump keys, keep frequently used actions near WASD, and avoid two important actions requiring the same finger at once.",
	VideoNotes:      "First test native monitor refresh, stable frame cap, low/medium shadows, and resolution/upscaling one step at a time. Benchmark each change.",
	ControllerNotes: "Start with default response curves and the smallest deadzone without drift; do not assume there is one best controller setup.", Binds: typicalFPSBinds,
}

func normalizeGameName(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return ' '
	}, s)
	return strings.Join(strings.Fields(s), " ")
}
func chooseGuide(name string) GameGuide {
	clean := normalizeGameName(name)
	if clean == "" {
		return genericGuide
	}
	for _, g := range gameGuides {
		for _, a := range g.Aliases {
			if strings.Contains(clean, normalizeGameName(a)) {
				return g
			}
		}
	}
	return genericGuide
}
func allGuides() []GameGuide { return append([]GameGuide{genericGuide}, gameGuides...) }

var explainers = []Explainer{
	{"scan", "Scan", "Automatic hardware & game scan", "Reads your PC's parts, video drivers, memory, disks and installed game manifests.", "Asks Windows for hardware information and checks recognized launcher folders. The scan is read-only; it does not install drivers or edit your games.", "Gives better advice than a one-size-fits-all preset.", "Some hardware sensors and game libraries may not be visible; an absent game does not mean it isn't installed.", "Nothing to undo; the scan only reads information."},
	{"gamemode", "Windows", "Windows Game Mode", "Helps Windows prioritize game activity over some background work.", "Turns on a Windows gaming preference. It does not overclock your CPU or increase your GPU power limit.", "Can reduce interference from background tasks, especially on a busy computer.", "Improvements may be zero on some systems; background downloads or updates still need manual attention.", "Choose Restore Original Settings to revert values backed up by this app."},
	{"dvr", "Windows", "Background recording (Game DVR)", "Controls whether Windows saves gameplay in the background.", "Competitive preset turns off the Windows background capture preference. Your separate OBS/streaming setup is not changed.", "Avoids some recording overhead on lower-end PCs.", "You will not have Windows background clips while disabled. Creator/Balanced presets leave your existing capture choice alone.", "Use Restore Original Settings to put back your saved capture preference."},
	{"power", "Windows", "High Performance power plan", "Changes how aggressively Windows prefers responsiveness versus saving power.", "Asks Windows to activate the High Performance plan if your PC supports it. It does not lock your CPU at maximum speed or force overclocking.", "May help some PCs avoid sluggish power-state transitions.", "More power draw and fan noise; laptops can lose battery life, and some systems show little FPS improvement.", "Use Restore Original Settings to switch back to your saved power plan."},
	{"pointer", "Input", "Windows pointer speed", "Controls how far your desktop cursor moves for a given mouse movement.", "Changes the Windows cursor slider value, not the mouse sensor DPI. Many games use raw mouse input and ignore this slider.", "Useful if the Windows desktop cursor feels too fast or slow.", "This setting is not an FPS optimization, and changing it can disrupt your desktop muscle memory.", "Move the slider back or use Windows Mouse Settings. Note your original value before changing it."},
	{"dpi", "Input", "Hardware mouse DPI", "Measures how many mouse sensor counts are reported per inch of movement.", "Set DPI using your mouse's hardware button or manufacturer software; this app saves the value you enter but cannot verify the sensor's true DPI.", "Pair DPI with game sensitivity for repeatable aiming; 400, 800 and 1600 are common starting points.", "A DPI number alone doesn't determine accuracy. Polling at extreme rates can increase CPU overhead on older PCs.", "Change DPI back through the mouse's software; saved app values can be edited anytime."},
	{"edpi", "Input", "eDPI sensitivity calculator", "Combines mouse DPI and game-specific sensitivity so setups are easier to compare within the same game.", "Most games multiply DPI by an in-game slider, but some games use percentages or custom multipliers; this app labels each formula.", "Helps you compare your own control and faster setups without guessing.", "eDPI from two different games usually isn't comparable. Zoom/ADS settings can change aiming feel.", "Change the entered DPI or sensitivity; it never writes to your game."},
	{"presentmon", "Measurement", "PresentMon frame-time benchmark", "Measures how long each rendered game frame takes to reach presentation.", "With PresentMon installed separately, the app records frame time and calculates average FPS and slower-frame statistics.", "Shows if a tweak actually helped smoothness—not just peak FPS.", "Capture tools can have small overhead; compare the same scene at the same resolution with similar background load.", "End the capture or remove the report. It doesn't permanently change Windows settings."},
	{"low", "Measurement", "1% low FPS", "Represents performance during the slowest roughly 1% of measured frames.", "Calculated from frame times, not a fixed game counter. Lower frame-time spikes generally make movement feel smoother.", "Can reveal stutter hidden by a high average FPS.", "Short captures, loading screens and different test scenes can skew numbers.", "It's a measurement, not a setting you can switch on or off."},
	{"processes", "Troubleshooting", "Background process monitor", "Shows which running programs use the most RAM and other system resources.", "Reads Windows process information. A process is simply a running program or part of one, such as a browser tab or audio service.", "Helps identify avoidable load while playing games.", "High memory use isn't automatically bad; ending critical Windows or game anti-cheat processes can cause crashes.", "Nothing to undo; the list is read-only. Close unneeded apps normally from their own windows."},
	{"sfc", "Repair", "SFC /scannow", "Checks protected Windows files and replaces damaged copies when possible.", "Uses Microsoft's built-in System File Checker. It looks for missing or altered Windows system files, not game files.", "Can help when Windows features fail or system files have been corrupted.", "May take time and use disk/CPU. It is not a general FPS booster; some failures require DISM first.", "SFC automatically repairs system files; use Windows restore options if a repair creates a rare issue."},
	{"dism", "Repair", "DISM RestoreHealth", "Repairs the Windows component store that Windows uses as a source of repair files.", "Uses the official Windows DISM tool. SFC may work better afterward if the component store was damaged.", "Useful for Windows corruption or failed integrity checks.", "Can take a long time, use Windows Update, and consume CPU/disk. It will not make a healthy GPU faster.", "DISM repairs Windows components; if it causes an unexpected problem, use Windows recovery/restore options."},
	{"dns", "Network", "Flush DNS cache", "Forgets saved website/domain lookups so Windows can ask again.", "Clears cached name-to-IP records. It does not change your internet speed, server route, or game matchmaking region.", "May help when a server hostname changed and Windows is using an outdated address.", "May briefly delay the first connection to websites; it does not reliably lower gaming ping.", "No restoration needed; Windows rebuilds the cache automatically."},
	{"ping", "Network", "Ping test", "Sends small network probes to check basic round-trip response time.", "Measures delay and potential loss to the host you entered, such as a router or public server.", "Helps distinguish local Wi-Fi issues from remote connectivity issues.", "A public server's ping is not your in-game server ping; some servers block the probes.", "Nothing to undo; read-only network test."},
	{"drivers", "Drivers", "GPU / chipset drivers", "Drivers are the software bridge between Windows and computer hardware.", "The driver hub shows detected versions and opens official NVIDIA/AMD/Windows locations, without silently installing anything.", "A driver update may fix rendering errors, game compatibility, or stutter.", "Brand-new releases can occasionally introduce regressions. Use official websites; don't install random driver packs.", "Windows Device Manager or vendor installers may offer rollback; keeping your last known working version helps."},
	{"video", "Graphics", "Game graphics preset and frame cap", "Trades visual detail against how much work the GPU and CPU must do.", "Lowering shadows or effects reduces rendering load; a frame limit can stabilize frame pacing.", "Often produces bigger improvements on lower-end GPUs than registry tweaks.", "A very low resolution may reduce clarity, and an inappropriate limiter can add delay. Test visually and with a benchmark.", "Restore game settings manually using the game's own menu."},
	{"debloat", "Windows", "Windows debloat tools", "Third-party utilities that remove or disable selected Windows features and bundled apps.", "The app links to the upstream Win11Debloat GitHub page; it does not automatically download or execute scripts.", "May reduce clutter and some unnecessary background work.", "Aggressive scripts can remove apps or services that updates, games, drivers, anti-cheat or content creation use.", "Follow the project's documented rollback or Windows restore point procedures."},
	{"overclock", "Safety", "CPU/GPU overclocking", "Makes hardware run beyond typical factory clock or power settings.", "This build deliberately doesn't apply automatic voltage, curve or clock changes; those require model-specific safeguards and stress testing.", "Can improve performance if thermals, power and silicon quality allow it.", "Can cause crashes, corruption, heat and extra power use; on weak cooling it can reduce performance.", "Use motherboard/vendor utilities to return to default clocks and test stability."},
	{"ai", "Assistant", "SAWCHA AI troubleshooting", "Explains scan findings and helps you narrow down a gaming problem step by step.", "Offline mode uses local troubleshooting rules; online mode is an optional real language model connection you initiate using your own API key.", "Makes diagnostics understandable and helps prioritize likely bottlenecks.", "Suggestions are not guaranteed diagnoses; do not apply major changes without checking the effect or making a backup.", "The assistant never makes system changes itself. Close online mode or clear the key at any time."},
}

func (a *App) guidesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		bad(w, "GET required", 405)
		return
	}
	send(w, map[string]interface{}{"games": allGuides(), "explainers": explainers})
}

type reqSensitivity struct {
	GameID    string  `json:"gameID"`
	DPI       int     `json:"dpi"`
	Sens      float64 `json:"sens"`
	PollingHz int     `json:"pollingHz"`
}

func (a *App) sensitivityHandler(w http.ResponseWriter, r *http.Request) {
	var v reqSensitivity
	if e := decode(r, &v); e != nil {
		bad(w, e.Error(), 400)
		return
	}
	if len(v.GameID) < 1 || len(v.GameID) > 100 || v.DPI < 100 || v.DPI > 32000 || v.Sens <= 0 || v.Sens > 2000 || v.PollingHz < 125 || v.PollingHz > 8000 {
		bad(w, "Use a game, DPI 100–32000, sensitivity 0.001–2000, and polling 125–8000 Hz", 400)
		return
	}
	a.mu.Lock()
	if a.config.SensitivityProfiles == nil {
		a.config.SensitivityProfiles = map[string]SensitivitySetting{}
	}
	a.config.SensitivityProfiles[v.GameID] = SensitivitySetting{v.DPI, v.Sens, v.PollingHz}
	a.mu.Unlock()
	if e := a.persistConfig(); e != nil {
		bad(w, "Could not save settings: "+e.Error(), 500)
		return
	}
	send(w, map[string]bool{"ok": true})
}

type AdvisorRequest struct {
	Message         string `json:"message"`
	GameID          string `json:"gameID"`
	Mode            string `json:"mode"`
	APIKey          string `json:"apiKey"`
	IncludeHardware bool   `json:"includeHardware"`
}
type AdvisorAnswer struct {
	Mode   string `json:"mode"`
	Answer string `json:"answer"`
	Source string `json:"source"`
}

func (a *App) advisorHandler(w http.ResponseWriter, r *http.Request) {
	var v AdvisorRequest
	if e := decode(r, &v); e != nil {
		bad(w, e.Error(), 400)
		return
	}
	v.Message = strings.TrimSpace(v.Message)
	if len(v.Message) < 2 || len(v.Message) > 1200 {
		bad(w, "Message must contain 2–1200 characters", 400)
		return
	}
	if len(v.GameID) > 100 {
		bad(w, "Invalid game selection", 400)
		return
	}
	a.mu.RLock()
	hardware := a.hardware
	games := append([]Game(nil), a.games...)
	hist := append([]Benchmark(nil), a.history...)
	advice := append([]Advice(nil), a.advice...)
	a.mu.RUnlock()
	var gameName string
	for _, g := range games {
		if g.ID == v.GameID {
			gameName = g.Name
			break
		}
	}
	if gameName == "" {
		for _, g := range gameGuides {
			if g.ID == v.GameID {
				gameName = g.Title
				break
			}
		}
	}
	if v.Mode == "online" {
		if len(v.APIKey) < 15 || !strings.HasPrefix(v.APIKey, "sk-") {
			bad(w, "Enter a valid API key for online AI. Key is not stored.", 400)
			return
		}
		answer, err := askOnlineAI(r.Context(), v.Message, gameName, v.APIKey, v.IncludeHardware, hardware, advice)
		if err != nil {
			bad(w, "Online AI request failed: "+err.Error(), 502)
			return
		}
		send(w, AdvisorAnswer{"online", answer, "Online OpenAI model • request sent on your instruction"})
		return
	}
	send(w, AdvisorAnswer{"offline", offlineAnswer(v.Message, gameName, hardware, hist, advice), "Offline rule-based guidance • no internet request"})
}

func offlineAnswer(q, game string, h map[string]interface{}, history []Benchmark, adv []Advice) string {
	x := strings.ToLower(q)
	guide := chooseGuide(game)
	lead := "SAWCHA DIAGNOSTICS — "
	if game != "" {
		lead += "for " + game + "\n\n"
	} else {
		lead += "general gaming\n\n"
	}
	cpuName, _ := h["CPU"].(map[string]interface{})
	cpu, _ := cpuName["Name"].(string)
	var gpu string
	gs := asMaps(h["GPU"])
	if len(gs) > 0 {
		gpu, _ = gs[0]["Name"].(string)
	}
	status := ""
	if cpu != "" || gpu != "" {
		status = fmt.Sprintf("Detected PC: %s | %s.\n\n", safeText(cpu, 70), safeText(gpu, 70))
	}
	var tips []string
	switch {
	case containsAny(x, "stutter", "frametime", "frame time", "hitch", "choppy", "smooth", "1% low", "fps drop"):
		tips = []string{"1. Open FPS Benchmark and capture the same 60-second scene twice. Average FPS alone can hide frame spikes—watch 1% lows and the 99th-percentile frame time.", "2. Open Live Monitor while the game runs. If system RAM is almost full, close unneeded browser windows; if GPU usage stays near 100%, reduce shadows, effects or render resolution.", "3. Check Driver Hub for GPU updates, and Repair Lab → Stutter & Crash Events for display resets or hardware warnings.", "4. Test one change at a time. A stable frame cap often feels smoother than fluctuating maximum FPS."}
	case containsAny(x, "ping", "network", "packet", "internet", "wifi", "wi-fi", "latency", "disconnect"):
		tips = []string{"1. Use Network Manager to ping your router, then a public host. If only the Wi-Fi connection spikes, test Ethernet or move closer to the router.", "2. Close downloads, cloud backups and streams on your own network while testing.", "3. Check whether the problem happens in all games or only one region/server; public ping tests are not an in-game ping measurement.", "4. DNS flushing can fix a bad hostname cache but usually cannot reduce gaming latency."}
	case containsAny(x, "crash", "blue screen", "bsod", "freez", "restart", "driver", "gpu not"):
		tips = []string{"1. Stop any CPU/GPU overclock or undervolt and test at factory settings first.", "2. Open Repair Lab → Stutter & Crash Events and look for WHEA errors, display-driver resets or unexpected shutdowns.", "3. If Windows files seem damaged, run DISM RestoreHealth and then SFC, ideally outside gaming hours.", "4. Check GPU power connections, temperature and the official driver release. Record the exact error message before changing more settings."}
	case containsAny(x, "dpi", "sensitivity", "sens", "aim", "mouse", "polling"):
		tips = []string{"1. The app cannot read physical sensor DPI from standard Windows mouse APIs. Check the mouse's own software or DPI indicator and enter the value in DPI Lab.", "2. For 800 DPI, use the selected game's Balance setup as a starting point, then adjust sensitivity by 5–10% at a time.", "3. Windows pointer speed is separate from game sensitivity and is often ignored by raw-input games.", "4. Test using a repeatable 180° turn and small tracking adjustments; avoid changing DPI and game sensitivity at the same time."}
	case containsAny(x, "bind", "keyboard", "controller", "deadzone", "lean", "crouch", "button"):
		tips = []string{"1. Open Keybind Studio for per-game recommended actions, reasons and alternatives. Suggested binds are not automatically injected into the game.", "2. Choose a layout where you can strafe, crouch and use a main ability without awkward finger conflicts.", "3. For controller drift, raise the deadzone only until the stick stops moving by itself.", "4. Keep one setup for a few sessions before evaluating whether it works."}
	case containsAny(x, "overclock", "clock", "voltage", "heat", "temp", "thermal", "throttle"):
		tips = []string{"1. Monitor temperatures and utilization while the issue occurs. If performance gets worse as temperatures rise, check airflow, dust and cooling.", "2. Return to factory clocks before troubleshooting crashes. This app intentionally cannot auto-apply risky voltage or overclock values.", "3. A stable frame rate is more useful than a brief high benchmark result.", "4. Use your motherboard/GPU manufacturer's own utility for any manual tuning, in small tested increments."}
	case containsAny(x, "record", "obs", "stream", "create", "video", "editing"):
		tips = []string{"1. Use the Creator preset so the app does not turn off your Windows background capture preference.", "2. Prefer supported hardware encoding in your streaming software if that reduces CPU load; monitor the encoder and GPU usage.", "3. Check disk free space and write speed. Recording at high bitrates to a nearly full disk can cause dropped frames.", "4. Compare gaming frame times while recording versus while not recording."}
	default:
		tips = []string{"1. Start with the Startup Scan recommendations and note the exact game, symptom and when it starts.", "2. Run a baseline benchmark in the same game scene. Change one setting at a time so you can tell what helped.", "3. Inspect Live Monitor for CPU, GPU, RAM and heavy background programs while the problem happens.", "4. If the issue is a crash, use Repair Lab. If it is aiming or controls, open Keybind Studio and DPI Lab. If it is online lag, use Network Manager."}
	}
	if game != "" && guide.ID != "general" {
		tips = append(tips, "GAME-SPECIFIC TIP: "+guide.VideoNotes)
	}
	if len(history) > 0 && containsAny(x, "stutter", "fps", "frame", "benchmark") {
		b := history[len(history)-1]
		tips = append(tips, fmt.Sprintf("LAST SAVED BENCHMARK: %s averaged %.1f FPS with %.1f FPS 1%% lows. Compare in identical scenes rather than treating one run as proof.", b.Game, b.AvgFPS, b.Low1))
	}
	if len(adv) > 0 && containsAny(x, "scan", "recommend", "optimize") {
		tips = append(tips, "YOUR SCAN SAYS: "+adv[0].Title+" — "+adv[0].Body)
	}
	return lead + status + strings.Join(tips, "\n\n") + "\n\nFor a narrower diagnosis, tell me the game, what happens, and whether it started after a driver or Windows change."
}
func containsAny(s string, terms ...string) bool {
	for _, t := range terms {
		if strings.Contains(s, t) {
			return true
		}
	}
	return false
}

var whitespace = regexp.MustCompile(`\s+`)

func safeText(s string, n int) string {
	s = whitespace.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	if len(s) > n {
		s = s[:n]
	}
	return s
}

func askOnlineAI(ctx context.Context, question, game, key string, share bool, h map[string]interface{}, adv []Advice) (string, error) {
	summary := "No hardware information shared."
	if share {
		c := asMaps(h["CPU"])
		g := asMaps(h["GPU"])
		o := asMaps(h["OS"])
		parts := []string{}
		if len(c) > 0 {
			parts = append(parts, "CPU "+safeText(fmt.Sprint(c[0]["Name"]), 100))
		}
		if len(g) > 0 {
			parts = append(parts, "GPU "+safeText(fmt.Sprint(g[0]["Name"]), 100))
		}
		if len(o) > 0 {
			parts = append(parts, "OS "+safeText(fmt.Sprint(o[0]["Caption"]), 70))
		}
		if len(adv) > 0 {
			parts = append(parts, "Scan note: "+safeText(adv[0].Title+": "+adv[0].Body, 240))
		}
		sort.Strings(parts)
		summary = strings.Join(parts, "; ")
	}
	instruction := "You are SAWCHA Optimizer's practical Windows 10/11 gaming & content creation troubleshooter. Explain terms simply to a nonprogrammer. Distinguish proven effects from possible improvements, never promise FPS gains. Never suggest bypassing anticheat, disabling security tools, unsafe debloat scripts, or risky blind overclocks. Never claim you can change Windows settings or read current mouse sensor DPI. Give 3-6 prioritized diagnostic steps and how to verify. Avoid unnecessary registry edits. Be concise and friendly."
	payload := map[string]interface{}{"model": "gpt-4.1-mini", "instructions": instruction, "input": fmt.Sprintf("Selected game: %s\nHardware info: %s\nUser question: %s", safeText(game, 110), summary, question), "max_output_tokens": 700, "store": false}
	encoded, e := json.Marshal(payload)
	if e != nil {
		return "", e
	}
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	request, e := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/responses", bytes.NewReader(encoded))
	if e != nil {
		return "", e
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 35 * time.Second}
	response, e := client.Do(request)
	if e != nil {
		return "", errors.New("could not connect to the AI service; check your internet connection")
	}
	defer response.Body.Close()
	body, e := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if e != nil {
		return "", errors.New("could not read AI response")
	}
	if response.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d (check API key, model access or API billing)", response.StatusCode)
	}
	var result struct {
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if e := json.Unmarshal(body, &result); e != nil {
		return "", errors.New("AI returned an unexpected response")
	}
	output := []string{}
	for _, p := range result.Output {
		for _, v := range p.Content {
			if v.Type == "output_text" && v.Text != "" {
				output = append(output, v.Text)
			}
		}
	}
	if len(output) == 0 {
		return "", errors.New("AI returned no text")
	}
	return strings.TrimSpace(strings.Join(output, "\n")), nil
}
