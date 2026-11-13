package audioengine

import (
  "os"
  "path/filepath"
  "strings"
  "testing"
)
func makeConfig(t *testing.T)string {
 t.Helper();d:=t.TempDir()
 if err:=os.WriteFile(filepath.Join(d,"config.txt"),[]byte("# user's existing EQ\nPreamp: -2 dB\n"),0600);err!=nil{t.Fatal(err)}
 return d
}
func TestAPO18BandsAndPreamp(t *testing.T) {
 p,_:=Preset("Footsteps")
 txt,err:=APOText(p,"Speakers")
 if err!=nil{t.Fatal(err)}
 if got:=strings.Count(txt,"ON PK Fc");got!=18{t.Fatalf("expected 18 filters, got %d",got)}
 if !strings.Contains(txt,"Preamp: -4.0 dB")||!strings.Contains(txt,"Device: Speakers") {t.Fatal("missing preamp or target device")}
 if _,err=APOText(p,"Speakers\nInclude: attack.txt");err==nil{t.Fatal("filter injection accepted")}
}
func TestApplyAPOPreservesExistingSettings(t *testing.T) {
 d:=makeConfig(t);entry:=filepath.Join(d,"config.txt")
 original,_:=os.ReadFile(entry)
 p,_:=Preset("Footsteps")
 if err:=ApplyAPO(d,p,"Speakers");err!=nil{t.Fatal(err)}
 first,_:=os.ReadFile(entry)
 if !strings.HasPrefix(string(first),string(original))||strings.Count(string(first),"Include: SAWCHA_EQ.txt")!=1 {
   t.Fatalf("existing EQ changed or duplicate include: %q",first)
 }
 if err:=ApplyAPO(d,p,"Speakers");err!=nil{t.Fatal(err)}
 second,_:=os.ReadFile(entry)
 if string(first)!=string(second){t.Fatal("reapplying EQ changed the entry config")}
 if err:=RemoveAPO(d);err!=nil{t.Fatal(err)}
 restored,_:=os.ReadFile(entry)
 if string(restored)!=string(original){t.Fatalf("rollback did not restore old settings: %q",restored)}
}
func TestNeverOverwriteUnmanagedEQ(t *testing.T) {
 d:=makeConfig(t)
 if err:=os.WriteFile(filepath.Join(d,"SAWCHA_EQ.txt"),[]byte("# existing unrelated configuration"),0600);err!=nil{t.Fatal(err)}
 p,_:=Preset("Flat")
 if err:=ApplyAPO(d,p,"");err==nil{t.Fatal("unmanaged file was overwritten")}
}
func TestNoApplyWhenAPOAbsent(t *testing.T) {
 p,_:=Preset("Flat")
 if err:=ApplyAPO(t.TempDir(),p,"");err==nil{t.Fatal("missing APO config unexpectedly accepted")}
}
