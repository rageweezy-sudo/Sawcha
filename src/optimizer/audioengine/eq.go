// Package audioengine provides testable, user-mode audio DSP primitives.
// It does not install devices, drivers, or capture system audio.
package audioengine

import (
    "errors"
    "math"
)

const Bands = 18

var FrequenciesHz = [Bands]float64{32,45,63,90,125,180,250,350,500,700,1000,1400,2000,2800,4000,5600,8000,12000}

// EQConfig is suitable for JSON serialization and local preset storage.
type EQConfig struct {
    Enabled bool `json:"enabled"`
    Name string `json:"name"`
    PreampDB float64 `json:"preampDb"`
    GainsDB [Bands]float64 `json:"gainsDb"`
}

func Preset(name string) (EQConfig, error) {
    p:=EQConfig{Enabled:true,Name:name,PreampDB:-4}
    switch name {
    case "Flat":
        p.PreampDB=0
    case "Footsteps":
        p.GainsDB=[Bands]float64{-3,-3,-3,-2,-2,-2,-1,0,1,1,2,2,2.5,3,3,2,1,0}
    case "Competitive FPS":
        p.GainsDB=[Bands]float64{-4,-4,-3,-3,-2,-2,-1,0,1,2,2,2,3,3,2,2,0,-1}
    case "Balanced":
        p.GainsDB=[Bands]float64{1,1,1,0,0,-1,-1,0,0,0,1,1,1,1,0,0,1,1}
    case "Music":
        p.GainsDB=[Bands]float64{3,3,2,2,1,0,-1,-1,0,0,0,1,1,1,2,2,2,2}
    default:
        return EQConfig{},errors.New("unknown EQ preset")
    }
    return p,nil
}

func (c EQConfig) Validate() error {
    if math.IsNaN(c.PreampDB)||math.IsInf(c.PreampDB,0)||c.PreampDB < -24||c.PreampDB > 6 {
        return errors.New("preamp must be finite and between -24 and +6 dB")
    }
    for _,gain:=range c.GainsDB {
        if math.IsNaN(gain)||math.IsInf(gain,0)||gain < -12||gain > 12 {
            return errors.New("EQ bands must be finite and between -12 and +12 dB")
        }
    }
    return nil
}

type biquad struct {
    b0,b1,b2,a1,a2 float64
    z1,z2 float64
}
func (b *biquad) process(x float64) float64 {
    y:=b.b0*x+b.z1
    b.z1=b.b1*x-b.a1*y+b.z2
    b.z2=b.b2*x-b.a2*y
    return y
}
func peakFilter(sampleRate,hz,gainDB float64) biquad {
    // RBJ peaking filter. Q reduces adjacent band overlap.
    const q=2.6
    a:=math.Pow(10,gainDB/40)
    w:=2*math.Pi*hz/sampleRate
    alpha:=math.Sin(w)/(2*q)
    c:=math.Cos(w)
    a0:=1+alpha/a
    return biquad{
        b0:(1+alpha*a)/a0,
        b1:(-2*c)/a0,
        b2:(1-alpha*a)/a0,
        a1:(-2*c)/a0,
        a2:(1-alpha/a)/a0,
    }
}

type Equalizer struct {
    cfg EQConfig
    channels int
    filters [][]biquad
    preamp float64
}
func NewEqualizer(sampleRate,channels int,config EQConfig) (*Equalizer,error) {
    if sampleRate<32000||sampleRate>192000||channels<1||channels>8 {
        return nil,errors.New("unsupported sample rate or channel count")
    }
    if err:=config.Validate();err!=nil {return nil,err}
    e:=&Equalizer{cfg:config,channels:channels,preamp:math.Pow(10,config.PreampDB/20)}
    e.filters=make([][]biquad,channels)
    for ch:=range e.filters {
        e.filters[ch]=make([]biquad,0,Bands)
        for i,hz:=range FrequenciesHz {
            // Zero-gain filters are exact identities. Skipping them reduces
            // work per frame and keeps the Flat preset allocation-free in DSP.
            if config.GainsDB[i]==0 {continue}
            e.filters[ch]=append(e.filters[ch],peakFilter(float64(sampleRate),hz,config.GainsDB[i]))
        }
    }
    return e,nil
}
// ProcessInterleaved modifies float PCM in place without per-call allocations.
// Use one processor per stream and call from only one audio thread.
// The caller handles device routing, sample conversion and final clip protection.
func (e *Equalizer) ProcessInterleaved(samples []float32) error {
    if len(samples)%e.channels!=0{return errors.New("interleaved buffer length is not divisible by channel count")}
    if !e.cfg.Enabled{return nil}
    for i,sample:=range samples {
        ch:=i%e.channels
        v:=float64(sample)*e.preamp
        for j:=range e.filters[ch] {v=e.filters[ch][j].process(v)}
        samples[i]=float32(v)
    }
    return nil
}
