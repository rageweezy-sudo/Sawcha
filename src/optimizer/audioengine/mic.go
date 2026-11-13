package audioengine

import (
    "errors"
    "math"
)

// MicConfig supports a soft speech expander, compressor and peak ceiling.
// This is not a device reconnect or hardware dropout repair algorithm.
type MicConfig struct {
    Enabled bool `json:"enabled"`
    HighPassHz float64 `json:"highPassHz"`
    ExpandBelowDB float64 `json:"expandBelowDb"`
    MaxReductionDB float64 `json:"maxReductionDb"`
    CompressorThresholdDB float64 `json:"compressorThresholdDb"`
    CompressorRatio float64 `json:"compressorRatio"`
    MakeupGainDB float64 `json:"makeupGainDb"`
    LimiterCeilingDB float64 `json:"limiterCeilingDb"`
}
func VoicePreset() MicConfig {
    return MicConfig{Enabled:true,HighPassHz:80,ExpandBelowDB:-48,MaxReductionDB:10,
        CompressorThresholdDB:-20,CompressorRatio:2.5,MakeupGainDB:3,LimiterCeilingDB:-1}
}
func (c MicConfig) Validate() error {
    vals:=[]float64{c.HighPassHz,c.ExpandBelowDB,c.MaxReductionDB,c.CompressorThresholdDB,c.CompressorRatio,c.MakeupGainDB,c.LimiterCeilingDB}
    for _,v:=range vals {if math.IsNaN(v)||math.IsInf(v,0){return errors.New("microphone settings must be finite")}}
    if c.HighPassHz<20||c.HighPassHz>300||c.ExpandBelowDB < -90||c.ExpandBelowDB > -20||
        c.MaxReductionDB<0||c.MaxReductionDB>24||c.CompressorThresholdDB < -45||c.CompressorThresholdDB > -5||
        c.CompressorRatio<1||c.CompressorRatio>10||c.MakeupGainDB < -12||c.MakeupGainDB>18||
        c.LimiterCeilingDB < -12||c.LimiterCeilingDB>0 {
        return errors.New("microphone setting outside supported range")
    }
    return nil
}
type MicProcessor struct {
    cfg MicConfig
    hpAlpha,prevInput,prevHighPass float64
    envelope,gain float64
    attack,release float64
    makeup,ceiling float64
}
func NewMicProcessor(sampleRate int,cfg MicConfig) (*MicProcessor,error) {
    if sampleRate<16000||sampleRate>192000 {return nil,errors.New("unsupported microphone sample rate")}
    if err:=cfg.Validate();err!=nil{return nil,err}
    dt:=1/float64(sampleRate)
    rc:=1/(2*math.Pi*cfg.HighPassHz)
    return &MicProcessor{cfg:cfg,hpAlpha:rc/(rc+dt),attack:math.Exp(-dt/0.010),
        release:math.Exp(-dt/0.140),gain:1,makeup:math.Pow(10,cfg.MakeupGainDB/20),
        ceiling:math.Pow(10,cfg.LimiterCeilingDB/20)},nil
}
// ProcessMono is a user-mode, in-place voice DSP. It does not capture,
// route, denoise or echo-cancel audio. A soft expander avoids a hard gate.
func (m *MicProcessor) ProcessMono(samples []float32) {
    if !m.cfg.Enabled{return}
    for i,x:=range samples {
        input:=float64(x)
        hp:=m.hpAlpha*(m.prevHighPass+input-m.prevInput)
        m.prevInput,m.prevHighPass=input,hp
        amp:=math.Abs(hp)
        smoothing:=m.release
        if amp>m.envelope{smoothing=m.attack}
        m.envelope=smoothing*m.envelope+(1-smoothing)*amp
        levelDB:=20*math.Log10(math.Max(m.envelope,1e-9))
        reductionDB:=0.0
        if levelDB<m.cfg.ExpandBelowDB{
            reductionDB=math.Min(m.cfg.MaxReductionDB,(m.cfg.ExpandBelowDB-levelDB)*0.5)
        }else if levelDB>m.cfg.CompressorThresholdDB{
            reductionDB=(levelDB-m.cfg.CompressorThresholdDB)*(1-1/m.cfg.CompressorRatio)
        }
        desired:=math.Pow(10,-reductionDB/20)
        factor:=m.release
        if desired<m.gain{factor=m.attack}
        m.gain=factor*m.gain+(1-factor)*desired
        output:=hp*m.gain*m.makeup
        if output>m.ceiling{output=m.ceiling}else if output < -m.ceiling{output=-m.ceiling}
        samples[i]=float32(output)
    }
}
