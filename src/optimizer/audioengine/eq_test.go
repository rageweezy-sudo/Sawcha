package audioengine

import (
    "math"
    "testing"
)

func TestPresetHasEighteenBandsAndHeadroom(t *testing.T) {
    for _,name:=range []string{"Flat","Footsteps","Competitive FPS","Balanced","Music"} {
        p,err:=Preset(name)
        if err!=nil{t.Fatal(err)}
        if err:=p.Validate();err!=nil{t.Fatalf("%s: %v",name,err)}
        if name!="Flat"&&p.PreampDB>=0{t.Errorf("%s needs preamp headroom",name)}
    }
    if _,err:=Preset("unknown");err==nil{t.Fatal("unknown preset accepted")}
}
func TestFlatIsIdentity(t *testing.T) {
    p,_:=Preset("Flat")
    eq,err:=NewEqualizer(48000,2,p)
    if err!=nil{t.Fatal(err)}
    samples:=[]float32{0.1,-0.2,0.3,-0.4,0,0.2}
    original:=append([]float32(nil),samples...)
    if err:=eq.ProcessInterleaved(samples);err!=nil{t.Fatal(err)}
    for i,v:=range samples{if math.Abs(float64(v-original[i]))>0.000001{t.Fatalf("sample %d changed: %v",i,v)}}
}
func TestInvalidEQRejected(t *testing.T) {
    p,_:=Preset("Flat");p.GainsDB[3]=math.NaN()
    if _,err:=NewEqualizer(48000,2,p);err==nil{t.Fatal("NaN gain accepted")}
    p,_=Preset("Flat")
    if _,err:=NewEqualizer(22050,2,p);err==nil{t.Fatal("sample rate below highest band accepted")}
}
func TestBoostAtOneKilohertz(t *testing.T) {
    p,_:=Preset("Flat");p.GainsDB[10]=6
    eq,err:=NewEqualizer(48000,1,p)
    if err!=nil{t.Fatal(err)}
    samples:=make([]float32,48000)
    var inEnergy float64
    for i:=range samples{samples[i]=0.1*float32(math.Sin(2*math.Pi*1000*float64(i)/48000));inEnergy+=float64(samples[i])*float64(samples[i])}
    if err:=eq.ProcessInterleaved(samples);err!=nil{t.Fatal(err)}
    var outEnergy float64
    for _,v:=range samples{outEnergy+=float64(v)*float64(v)}
    db:=10*math.Log10(outEnergy/inEnergy)
    if db<4.5||db>7{t.Fatalf("expected ~6 dB at 1 kHz, got %.2f",db)}
}
func TestMalformedInterleavedRejected(t *testing.T) {
    p,_:=Preset("Flat");eq,_:=NewEqualizer(48000,2,p)
    if err:=eq.ProcessInterleaved([]float32{0.1});err==nil{t.Fatal("odd stereo buffer accepted")}
}
