package audioengine

import (
    "math"
    "testing"
)

func TestVoiceConfigValidation(t *testing.T) {
    c:=VoicePreset()
    if err:=c.Validate();err!=nil{t.Fatal(err)}
    c.CompressorRatio=0
    if _,err:=NewMicProcessor(48000,c);err==nil{t.Fatal("invalid ratio accepted")}
}
func TestMicLimiterHasSafeCeiling(t *testing.T) {
    m,err:=NewMicProcessor(48000,VoicePreset());if err!=nil{t.Fatal(err)}
    buf:=make([]float32,4800)
    for i:=range buf{buf[i]=float32(2*math.Sin(2*math.Pi*500*float64(i)/48000))}
    m.ProcessMono(buf)
    limit:=math.Pow(10,-1./20)
    for _,v:=range buf{if math.Abs(float64(v))>limit+0.000001{t.Fatal("sample exceeds limiter ceiling")}}
}
func TestMicBypassPreservesSamples(t *testing.T) {
    cfg:=VoicePreset();cfg.Enabled=false
    m,err:=NewMicProcessor(48000,cfg);if err!=nil{t.Fatal(err)}
    samples:=[]float32{0,0.1,-0.1,0.5}
    m.ProcessMono(samples)
    if samples[1]!=0.1||samples[2]!=-0.1{t.Fatal("bypass changed samples")}
}
func TestMicDoesNotHardMuteQuietSpeech(t *testing.T) {
    m,err:=NewMicProcessor(48000,VoicePreset());if err!=nil{t.Fatal(err)}
    buf:=make([]float32,9600)
    for i:=range buf{buf[i]=float32(0.002*math.Sin(2*math.Pi*250*float64(i)/48000))}
    m.ProcessMono(buf)
    if math.Abs(float64(buf[9000]))<0.00001{t.Fatal("quiet speech was hard muted")}
}
