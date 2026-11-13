package main

import (
 "errors"
 "fmt"
 "net/http"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strconv"
 "strings"

 "sawcha/optimizer/audioengine"
)

// Audio selection is a local setup preference, NOT a server-verified
// subscription entitlement. Pro-only paid processing remains unavailable
// until the separate licensing system explicitly authorizes it.
type AudioSettings struct {
 Plan string `json:"plan"`
 Equalizer bool `json:"equalizer"`
 Microphone bool `json:"microphone"`
 DevicePattern string `json:"devicePattern"`
 EQ audioengine.EQConfig `json:"eq"`
}
func defaultAudioSettings() AudioSettings {
 eq,_:=audioengine.Preset("Flat")
 return AudioSettings{Plan:"Basic",Equalizer:true,EQ:eq}
}
func (s AudioSettings) validate() error {
 if s.Plan!="Basic" && s.Plan!="Pro"{return errors.New("audio plan must be Basic or Pro")}
 if err:=s.EQ.Validate();err!=nil{return err}
 if err:=audioengine.ValidateDevicePattern(s.DevicePattern);err!=nil{return err}
 return nil
}

func discoverAPO() (string,string) {
 if runtime.GOOS!="windows" {return "", "Windows 10 or 11 required"}
 if runtime.GOARCH!="amd64" && runtime.GOARCH!="386" && runtime.GOARCH!="arm64" {
   return "","unsupported Windows processor architecture"
 }
 // Registry contains the configured Equalizer APO config folder, including
 // user-chosen nondefault installation locations.
 cmd:=exec.Command("reg.exe","query",`HKLM\SOFTWARE\EqualizerAPO`,"/v","ConfigPath")
 prepareHidden(cmd)
 b,err:=cmd.CombinedOutput()
 if err==nil {
   for _,line:=range strings.Split(string(b),"\n") {
     items:=strings.Fields(strings.TrimSpace(line))
     if len(items)>=3&&strings.EqualFold(items[0],"ConfigPath") && strings.HasPrefix(items[1],"REG_") {
       dir:=strings.Join(items[2:]," ")
       if isAPOConfigDir(dir){return dir,""}
     }
   }
 }
 if root:=os.Getenv("ProgramFiles");root!="" {
   dir:=filepath.Join(root,"EqualizerAPO","config")
   if isAPOConfigDir(dir){return dir,""}
 }
 return "","Equalizer APO is not installed or its configuration cannot be located. Install from the official SourceForge website and select the output device in its Device Selector."
}
func isAPOConfigDir(dir string) bool {
 if dir=="" {return false}
 entry:=filepath.Join(dir,"config.txt")
 f,err:=os.Stat(entry)
 if err!=nil||f.IsDir(){return false}
 return true
}
func (a *App) audioSettingsHandler(w http.ResponseWriter,r *http.Request) {
 var request struct {
  Settings AudioSettings `json:"settings"`
 }
 if err:=decode(r,&request);err!=nil{bad(w,err.Error(),400);return}
 if err:=request.Settings.validate();err!=nil{bad(w,err.Error(),400);return}
 a.mu.Lock()
 a.config.Audio=request.Settings
 a.mu.Unlock()
 if err:=a.persistConfig();err!=nil{bad(w,err.Error(),500);return}
 send(w,map[string]interface{}{"ok":true,"settings":request.Settings,"notice":"Pro selection is not a subscription entitlement. Microphone processing requires a separate audio routing engine."})
}
func (a *App) audioStatusHandler(w http.ResponseWriter,r *http.Request) {
 if r.Method!="POST"{bad(w,"POST required",405);return}
 a.mu.RLock();settings:=a.config.Audio;a.mu.RUnlock()
 dir,problem:=discoverAPO()
 enabled,managed:=false,false
 if dir!="" {
   var err error
   enabled,managed,err=audioengine.ReadAPOStatus(dir)
   if err!=nil {problem=err.Error()}
 }
 send(w,map[string]interface{}{
  "settings":settings,"windows":runtime.GOOS=="windows","architecture":runtime.GOARCH,
  "backend":"Equalizer APO","configDetected":dir!="","configDir":dir,
  "includedInConfig":enabled,"managedFiltersPresent":managed,
  "problem":problem,
  "microphoneRoutingReady":false,
  "proAudioEntitlementVerified":false,
  "note":"An active Include is not proof that the driver is attached or audio effects are audible. Verify the output device in Equalizer APO Device Selector and test sound.",
 })
}
func (a *App) audioApplyHandler(w http.ResponseWriter,r *http.Request) {
 var req struct{AckGameRunning bool `json:"ack_game_running"`}
 if err:=decode(r,&req);err!=nil{bad(w,err.Error(),400);return}
 game,prefs:=a.guardSnapshot()
 if game!=""&&prefs.WarnBeforeChanges&&!req.AckGameRunning {
   bad(w,"Game is running; confirm that a short audio interruption is acceptable",409);return
 }
 a.mu.RLock();s:=a.config.Audio;a.mu.RUnlock()
 if !s.Equalizer{bad(w,"Enable the EQ module in Audio Setup first",400);return}
 if err:=s.validate();err!=nil{bad(w,err.Error(),400);return}
 dir,problem:=discoverAPO()
 if problem!="" {bad(w,problem,400);return}
 if err:=audioengine.ApplyAPO(dir,s.EQ,s.DevicePattern);err!=nil{bad(w,"Unable to activate Equalizer APO EQ: "+err.Error(),500);return}
 send(w,map[string]interface{}{"ok":true,"configWritten":true,"deviceVerified":false,"message":"EQ configuration installed. Ensure Equalizer APO is attached to the intended playback device and verify audibly; reboot if newly installed."})
}
func (a *App) audioRemoveHandler(w http.ResponseWriter,r *http.Request) {
 var req struct{AckGameRunning bool `json:"ack_game_running"`}
 if err:=decode(r,&req);err!=nil{bad(w,err.Error(),400);return}
 game,prefs:=a.guardSnapshot()
 if game!=""&&prefs.WarnBeforeChanges&&!req.AckGameRunning {bad(w,"Game is running; confirm before restoring EQ",409);return}
 dir,problem:=discoverAPO()
 if problem!=""{bad(w,problem,400);return}
 if err:=audioengine.RemoveAPO(dir);err!=nil{bad(w,"Unable to remove SAWCHA EQ: "+err.Error(),500);return}
 send(w,map[string]interface{}{"ok":true,"message":"Removed SAWCHA EQ Include. Other Equalizer APO user settings were retained."})
}
func (a *App) audioPresetHandler(w http.ResponseWriter,r *http.Request) {
 var req struct{Name string `json:"name"`}
 if err:=decode(r,&req);err!=nil{bad(w,err.Error(),400);return}
 p,err:=audioengine.Preset(req.Name)
 if err!=nil{bad(w,err.Error(),400);return}
 a.mu.Lock();s:=a.config.Audio;s.EQ=p;a.config.Audio=s;a.mu.Unlock()
 if err=a.persistConfig();err!=nil{bad(w,err.Error(),500);return}
 send(w,map[string]interface{}{"ok":true,"eq":p,"message":"Preset saved in SAWCHA. Select APPLY TO WINDOWS to update Equalizer APO."})
}
func (a *App) audioBandHandler(w http.ResponseWriter,r *http.Request) {
 var req struct{Index int `json:"index"`; Value float64 `json:"value"`; Preamp bool `json:"preamp"`}
 if err:=decode(r,&req);err!=nil{bad(w,err.Error(),400);return}
 a.mu.Lock()
 s:=a.config.Audio
 if req.Preamp {
   s.EQ.PreampDB=req.Value
 } else {
   if req.Index<0||req.Index>=audioengine.Bands {a.mu.Unlock();bad(w,"EQ band index outside 0 to 17",400);return}
   s.EQ.GainsDB[req.Index]=req.Value
 }
 if err:=s.EQ.Validate();err!=nil{a.mu.Unlock();bad(w,err.Error(),400);return}
 s.EQ.Name="Custom";s.EQ.Enabled=true
 a.config.Audio=s;a.mu.Unlock()
 if err:=a.persistConfig();err!=nil{bad(w,err.Error(),500);return}
 send(w,map[string]interface{}{"ok":true,"eq":s.EQ,"note":"Not applied to Windows until APPLY TO WINDOWS is selected."})
}
// audioVendorTool starts only a verified path inside the discovered install
// directory. It never executes installer binaries from third-party downloads.
func audioVendorTool(kind string) error {
 dir,problem:=discoverAPO();if problem!=""{return errors.New(problem)}
 base:=filepath.Dir(dir)
 names:=[]string{"DeviceSelector.exe","Configurator.exe"}
 if kind=="editor" {names=[]string{"Editor.exe"}}
 for _,name:=range names{
   path:=filepath.Join(base,name)
   if _,err:=os.Stat(path);err==nil {
     return exec.Command(path).Start()
   }
 }
 return fmt.Errorf("Equalizer APO %s was not found alongside the installation",kind)
}
var _ = strconv.Itoa
