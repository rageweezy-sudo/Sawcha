package audioengine

import (
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "regexp"
 "strings"
)

// APO integration uses the separately installed Equalizer APO, never a kernel
// driver or game injection. The device selector must be configured by the user.
const managedFile = "SAWCHA_EQ.txt"
const managedHeader = "# SAWCHA EQ - managed by Sawcha. Do not edit while Sawcha is running."
const beginMarker = "# SAWCHA EQ BEGIN"
const endMarker = "# SAWCHA EQ END"

var allowDevice = regexp.MustCompile(`^[^\x00-\x1f;]{1,160}$`)

func ValidateDevicePattern(device string) error {
 if device == "" { return nil } // empty means all APO-selected endpoints
 if !allowDevice.MatchString(device) { return errors.New("device filter contains invalid characters") }
 return nil
}

// APOText generates the exact Equalizer APO parser syntax for 18 PK bands.
// Empty device applies to every endpoint explicitly enabled in Device Selector.
func APOText(eq EQConfig, device string) (string,error) {
 if err:=eq.Validate();err!=nil{return "",err}
 if err:=ValidateDevicePattern(device);err!=nil{return "",err}
 var b strings.Builder
 b.WriteString(managedHeader+"\n")
 if strings.TrimSpace(device)=="" {b.WriteString("Device: all\n")} else {fmt.Fprintf(&b,"Device: %s\n",strings.TrimSpace(device))}
 b.WriteString("Channel: all\n")
 if !eq.Enabled {b.WriteString("# EQ bypassed\n");return b.String(),nil}
 if eq.PreampDB!=0 {fmt.Fprintf(&b,"Preamp: %.1f dB\n",eq.PreampDB)}
 active:=0
 for i, hz:=range FrequenciesHz {
    // Omit zero-gain filters entirely: Flat remains a true no-op.
    if eq.GainsDB[i]==0 {continue}
    active++
    fmt.Fprintf(&b,"Filter %d: ON PK Fc %.0f Hz Gain %.1f dB Q 2.60\n",i+1,hz,eq.GainsDB[i])
 }
 if active==0 && eq.PreampDB==0 {b.WriteString("# Flat profile: no filters or gain changes\n")}
 return b.String(),nil
}

// ReplaceFile writes via a temporary file and renames only on success. A
// separate backup ensures lossless restoration when possible.
func replaceFile(path string, data []byte) error {
 tmp,err:=os.CreateTemp(filepath.Dir(path),".sawcha-*.tmp")
 if err!=nil{return err}
 name:=tmp.Name()
 defer os.Remove(name)
 if _,err=tmp.Write(data);err!=nil{tmp.Close();return err}
 if err=tmp.Sync();err!=nil{tmp.Close();return err}
 if err=tmp.Close();err!=nil{return err}
 return os.Rename(name,path)
}

// ApplyAPO adds an Include at the end of an existing config, preserving all
// other lines. The first original config is backed up before any edit.
// It never downloads/install drivers and never overwrites an unknown managed file.
func ApplyAPO(configDir string,eq EQConfig,device string) error {
 text,err:=APOText(eq,device);if err!=nil{return err}
 entry:=filepath.Join(configDir,"config.txt")
 original,err:=os.ReadFile(entry)
 if err!=nil{return fmt.Errorf("Equalizer APO config.txt is not accessible: %w",err)}
 if len(original)>4<<20 {return errors.New("Equalizer APO configuration is unexpectedly large")}
 managed:=filepath.Join(configDir,managedFile)
 if previous,err:=os.ReadFile(managed);err==nil {
  if !strings.HasPrefix(string(previous),managedHeader+"\n"){return errors.New("SAWCHA_EQ.txt already exists and is not managed by Sawcha")}
 } else if !errors.Is(err,os.ErrNotExist){return err}
 prior:=string(original)
 if strings.Count(prior,beginMarker)!=strings.Count(prior,endMarker){return errors.New("partial SAWCHA block in Equalizer APO config: repair manually")}
 if strings.Count(prior,beginMarker)>1{return errors.New("multiple SAWCHA blocks found in Equalizer APO config")}
 if strings.Contains(prior,beginMarker) {
    expected:=beginMarker+"\nDevice: all\nInclude: "+managedFile+"\n"+endMarker
    if !strings.Contains(prior,expected){return errors.New("modified SAWCHA block; refusing to overwrite user changes")}
    // Reapplying edits only the owned filter file, never the user's entry config.
    return replaceFile(managed,[]byte(text))
 }
 // Check include collision before changing any file, even if marker is absent.
 if strings.Contains(strings.ToLower(prior),"include: sawcha_eq.txt") {
   return errors.New("an existing SAWCHA_EQ include outside the managed block was found")
 }
 backup:=entry+".sawcha-original.bak"
 if _,err=os.Stat(backup);errors.Is(err,os.ErrNotExist) {
    if err=replaceFile(backup,original);err!=nil{return fmt.Errorf("unable to backup Equalizer APO configuration: %w",err)}
 } else if err!=nil{return err}
 if err=replaceFile(managed,[]byte(text));err!=nil{return err}
 if prior!=""&&!strings.HasSuffix(prior,"\n"){prior+="\n"}
 prior+=beginMarker+"\nDevice: all\nInclude: "+managedFile+"\n"+endMarker+"\n"
 if err=replaceFile(entry,[]byte(prior));err!=nil{return fmt.Errorf("failed to activate SAWCHA EQ: %w",err)}
 return nil
}

// RemoveAPO removes only the SAWCHA-owned block. All non-SAWCHA lines are
// retained. Uninstall never uninstalls Equalizer APO or any audio driver.
func RemoveAPO(configDir string) error {
 entry:=filepath.Join(configDir,"config.txt")
 original,err:=os.ReadFile(entry);if err!=nil{return err}
 s:=string(original)
 if !strings.Contains(s,beginMarker){return nil}
 if strings.Count(s,beginMarker)!=1||strings.Count(s,endMarker)!=1{return errors.New("ambiguous SAWCHA markers; refusing to modify config")}
 expectedBlock:=beginMarker+"\nDevice: all\nInclude: "+managedFile+"\n"+endMarker
 if !strings.Contains(s,expectedBlock){return errors.New("SAWCHA block was edited; refusing to remove unknown settings")}
 before,after,_:=strings.Cut(s,beginMarker)
 _,suffix,ok:=strings.Cut(after,endMarker)
 if !ok{return errors.New("missing SAWCHA end marker")}
 // Restore exact original bytes when the entry file has not been edited
 // outside SAWCHA's inserted section. Otherwise remove only our block.
 s=before+strings.TrimPrefix(suffix,"\n")
 if backup,readErr:=os.ReadFile(entry+".sawcha-original.bak");readErr==nil {
   orig:=string(backup)
   expected:=orig
   if expected!=""&&!strings.HasSuffix(expected,"\n"){expected+="\n"}
   expected+=beginMarker+"\nDevice: all\nInclude: "+managedFile+"\n"+endMarker+"\n"
   if string(original)==expected {s=orig}
 }
 if err=replaceFile(entry,[]byte(s));err!=nil{return err}
 return nil
}

// ReadAPOStatus is advisory: hardware installation and audible results require
// validation in Equalizer APO Device Selector, and a reboot when first attached.
func ReadAPOStatus(configDir string)(bool,bool,error) {
 if configDir==""{return false,false,nil}
 b,err:=os.ReadFile(filepath.Join(configDir,"config.txt"))
 if err!=nil{return false,false,err}
 _,fileErr:=os.Stat(filepath.Join(configDir,managedFile))
 return strings.Contains(string(b),beginMarker),fileErr==nil,nil
}

