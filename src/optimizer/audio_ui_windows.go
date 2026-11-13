package main

import (
 "fmt"
 "strconv"
 "strings"
 "sawcha/optimizer/audioengine"
)

func (n *nativeApp) audioSnapshot() AudioSettings {
 n.app.mu.RLock()
 defer n.app.mu.RUnlock()
 return n.app.config.Audio
}
func (n *nativeApp) audioSave(settings AudioSettings) {
 n.async(func()error{
    _,err:=n.api("/api/audio/settings",map[string]interface{}{"settings":settings})
    if err==nil{n.note("Audio setup preferences saved. The EQ takes effect only after APPLY TO WINDOWS.")}
    return err
 })
}
func (n *nativeApp) audioAction(action string) {
 switch action {
 case "setup":n.pageTo("audio-setup");return
 case "studio":n.pageTo("audio");return
 case "download":
    n.openLink("https://sourceforge.net/projects/equalizerapo/")
    n.note("Opened the official Equalizer APO download page. Install it, choose the playback device, and reboot if prompted.")
    return
 case "selector","editor":
    n.async(func()error{return audioVendorTool(action)})
    return
 case "check":
    n.async(func()error{
       state,err:=n.api("/api/audio/status",map[string]string{})
       if err!=nil{return err}
       if state["configDetected"]!=true {n.note("Equalizer APO not detected. Install it, choose a playback device, then reboot.");return nil}
       if state["includedInConfig"]==true&&state["managedFiltersPresent"]==true {
           n.note("SAWCHA EQ file is linked to Equalizer APO. Verify device attachment in Device Selector and listen for a change.")
       }else{
           n.note("Equalizer APO found. Select playback device and choose INSTALL EQ PROFILE.")
       }
       return nil
    })
    return
 case "save":
    s:=n.audioSnapshot()
    s.DevicePattern=strings.TrimSpace(n.read("audiodevice"))
    n.audioSave(s)
    return
 case "Basic","Pro":
    s:=n.audioSnapshot()
    s.Plan=action
    n.audioSave(s)
    return
 case "toggleeq","togglemic":
    s:=n.audioSnapshot()
    if action=="toggleeq"{s.Equalizer=!s.Equalizer}else{s.Microphone=!s.Microphone}
    n.audioSave(s)
    return
 case "apply":
    if !n.confirm("Install SAWCHA EQ settings into your existing Equalizer APO configuration? A backup is retained. The audio device must already be selected in Equalizer APO.")||!n.gameWarning("Updating the Equalizer APO audio filters"){return}
    n.async(func()error{
        res,err:=n.api("/api/audio/apply",map[string]bool{"ack_game_running":true})
        if err==nil{n.note(fmt.Sprint(res["message"]))}
        return err
    })
    return
 case "remove":
    if !n.confirm("Remove only the SAWCHA EQ Include from Equalizer APO? All other EQ configuration and the driver remain installed.")||!n.gameWarning("Removing the SAWCHA equalizer"){return}
    n.async(func()error{
        res,err:=n.api("/api/audio/remove",map[string]bool{"ack_game_running":true})
        if err==nil{n.note(fmt.Sprint(res["message"]))}
        return err
    })
    return
 }
 if strings.HasPrefix(action,"preset:") {
    name:=strings.TrimPrefix(action,"preset:")
    n.async(func()error{
        _,err:=n.api("/api/audio/preset",map[string]string{"name":name})
        if err==nil{n.note("Preset "+name+" saved. Use APPLY TO WINDOWS to hear changes.")}
        return err
    })
    return
 }
 if strings.HasPrefix(action,"band:") {
    fields:=strings.Split(action,":")
    if len(fields)!=3{return}
    i,err:=strconv.Atoi(fields[1]);if err!=nil||i<0||i>=audioengine.Bands{return}
    delta,err:=strconv.ParseFloat(fields[2],64);if err!=nil{return}
    s:=n.audioSnapshot()
    value:=s.EQ.GainsDB[i]+delta
    if value>12{value=12};if value < -12{value = -12}
    n.async(func()error{
        _,err:=n.api("/api/audio/band",map[string]interface{}{"index":i,"value":value})
        if err==nil{n.note(fmt.Sprintf("%g Hz: %+.1f dB saved. APPLY TO WINDOWS to hear it.",audioengine.FrequenciesHz[i],value))}
        return err
    })
    return
 }
 if strings.HasPrefix(action,"gain:") {
    delta,err:=strconv.ParseFloat(strings.TrimPrefix(action,"gain:"),64);if err!=nil{return}
    s:=n.audioSnapshot()
    value:=s.EQ.PreampDB+delta
    if value>0{value=0};if value < -24{value = -24}
    n.async(func()error{
        _,err:=n.api("/api/audio/band",map[string]interface{}{"preamp":true,"value":value})
        if err==nil{n.note("Preamp updated. APPLY TO WINDOWS to hear the difference.")}
        return err
    })
 }
}
func (n *nativeApp) paintAudioStudio(dc uintptr,x,cw int) {
 n.heading(dc,"AUDIO STUDIO / 18-BAND EQ","Windows 10/11 / Intel or AMD x64 / Equalizer APO backend",x,106,cw)
 s:=n.audioSnapshot()
 n.panel(dc,x,190,cw,111)
 n.text(dc,"AUDIO PROFILE: "+s.EQ.Name+"  /  "+s.Plan,x+18,200,cw-35,30,red,n.fontBold,false)
 n.wrap(dc,"EQ values are saved locally. Only APPLY TO WINDOWS updates the actual Equalizer APO filter file.",x+18,228,cw-36,39)
 w:=(cw-42)/3
 n.button(dc,"audio:setup","SETUP WIZARD",x+14,266,w,34,false)
 n.button(dc,"audio:apply","APPLY TO WINDOWS",x+22+w,266,w,34,true)
 n.button(dc,"audio:remove","REMOVE EQ",x+30+2*w,266,w,34,false)
 n.text(dc,"GAME / MUSIC PRESETS (STARTING CURVES)",x,311,cw,28,red,n.fontBold,false)
 names:=[]string{"Flat","Footsteps","Competitive FPS","Balanced","Music"}
 bw:=(cw-36)/5
 for i,name:=range names {
    n.button(dc,"audio:preset:"+name,name,x+i*(bw+9),341,bw,36,name==s.EQ.Name)
 }
 n.panel(dc,x,387,cw,49)
 n.text(dc,fmt.Sprintf("PREAMP / HEADROOM   %+.1f dB",s.EQ.PreampDB),x+14,397,cw-150,31,white,n.fontBold,false)
 n.button(dc,"audio:gain:-0.5","-0.5",x+cw-136,395,59,34,false)
 n.button(dc,"audio:gain:0.5","+0.5",x+cw-70,395,59,34,false)
 n.text(dc,"18 PARAMETRIC BANDS   /   HALF-DECIBEL STEPS",x,443,cw,28,red,n.fontBold,false)
 width:=(cw-18)/3
 for i,freq:=range audioengine.FrequenciesHz {
    col,row:=i%3,i/3
    xx:=x+col*(width+9);yy:=477+row*47
    n.panel(dc,xx,yy,width,41)
    label:=fmt.Sprintf("%.0f Hz   %+.1f",freq,s.EQ.GainsDB[i])
    if freq>=1000{label=fmt.Sprintf("%gk Hz  %+.1f",freq/1000,s.EQ.GainsDB[i])}
    n.text(dc,label,xx+7,yy+3,width-86,35,white,n.fontSmall,false)
    n.button(dc,fmt.Sprintf("audio:band:%d:-0.5",i),"-",xx+width-76,yy+5,32,30,false)
    n.button(dc,fmt.Sprintf("audio:band:%d:0.5",i),"+",xx+width-39,yy+5,32,30,true)
 }
 n.wrap(dc,"No game injection. Equalizer APO must be installed on the intended playback endpoint. Exclusive-mode, ASIO and unsupported drivers may bypass system processing.",x,768,cw,44)
}
func (n *nativeApp) paintAudioSetup(dc uintptr,x,cw int) {
 n.heading(dc,"AUDIO SETUP WIZARD","Select modules, choose a playback device and enable the real EQ backend.",x,106,cw)
 s:=n.audioSnapshot()
 n.panel(dc,x,192,cw,110)
 n.text(dc,"STEP 1 / AUDIO EDITION",x+15,202,cw-30,27,red,n.fontBold,false)
 n.button(dc,"audio:Basic","BASIC / FREE",x+15,244,(cw-50)/2,40,s.Plan=="Basic")
 n.button(dc,"audio:Pro","PRO / SELECT",x+30+(cw-50)/2,244,(cw-50)/2,40,s.Plan=="Pro")
 n.panel(dc,x,315,cw,119)
 n.text(dc,"STEP 2 / COMPONENTS",x+15,321,cw-30,29,red,n.fontBold,false)
 toggle:=func(enabled bool)string{if enabled{return "SELECTED"};return "NOT SELECTED"}
 n.button(dc,"audio:toggleeq","18-BAND EQ: "+toggle(s.Equalizer),x+15,360,(cw-49)/2,40,s.Equalizer)
 n.button(dc,"audio:togglemic","MIC STUDIO: "+toggle(s.Microphone),x+26+(cw-49)/2,360,(cw-49)/2,40,false)
 n.wrap(dc,"Microphone routing and Pro voice effects are planned, not installed by this wizard. Pro selection does not grant a paid entitlement.",x+15,405,cw-30,29)
 n.panel(dc,x,448,cw,120)
 n.text(dc,"STEP 3 / INSTALL AND ATTACH EQUALIZER APO",x+15,455,cw-30,27,red,n.fontBold,false)
 n.wrap(dc,"Install from the official website, run DEVICE SELECTOR, select the playback device (headphones or speakers) and reboot if needed.",x+15,486,cw-30,38)
 w:=(cw-50)/3
 n.button(dc,"audio:download","DOWNLOAD",x+12,530,w,36,false)
 n.button(dc,"audio:selector","DEVICE SELECTOR",x+25+w,530,w,36,false)
 n.button(dc,"audio:check","CHECK STATUS",x+38+2*w,530,w,36,false)
 n.panel(dc,x,581,cw,110)
 n.text(dc,"STEP 4 / OPTIONAL AUDIO DEVICE FILTER",x+15,587,cw-30,27,red,n.fontBold,false)
 n.wrap(dc,"Type the playback device name (for example, Speakers), or leave blank to process every endpoint selected in Equalizer APO.",x+15,617,cw-30,40)
 n.button(dc,"audio:save","SAVE SETUP",x+cw-168,653,150,32,true)
 n.panel(dc,x,704,cw,100)
 n.text(dc,"STEP 5 / APPLY OR ROLLBACK",x+15,710,cw-30,29,red,n.fontBold,false)
 n.button(dc,"audio:apply","INSTALL EQ PROFILE",x+15,751,(cw-50)/3,39,true)
 n.button(dc,"audio:remove","REMOVE SAWCHA EQ",x+25+(cw-50)/3,751,(cw-50)/3,39,false)
 n.button(dc,"audio:studio","BACK TO EQ",x+35+2*(cw-50)/3,751,(cw-50)/3,39,false)
}
