package main

import (
 "bytes"
 "encoding/json"
 "net/http/httptest"
 "testing"
)
func TestAudioDefaultsAndPlans(t *testing.T) {
 s:=defaultAudioSettings()
 if s.Plan!="Basic"||!s.Equalizer||s.EQ.Name!="Flat" {t.Fatal("unsafe audio defaults")}
 if err:=s.validate();err!=nil{t.Fatal(err)}
 s.Plan="Pro"
 if err:=s.validate();err!=nil{t.Fatal(err)}
 s.Plan="free-pro-unlocked"
 if err:=s.validate();err==nil{t.Fatal("unrecognized plan accepted")}
}
func TestAudioRejectsBadBandAndDeviceInput(t *testing.T) {
 s:=defaultAudioSettings();s.EQ.GainsDB[0]=23
 if err:=s.validate();err==nil{t.Fatal("bad EQ gain accepted")}
 s=defaultAudioSettings();s.DevicePattern="Speakers\nInclude: bad.txt"
 if err:=s.validate();err==nil{t.Fatal("device filter injection accepted")}
}
func TestAudioStatusDoesNotClaimDriverAttached(t *testing.T) {
 a:=&App{config:SavedConfig{Audio:defaultAudioSettings()}}
 r:=httptest.NewRequest("POST","/api/audio/status",bytes.NewBufferString("{}"))
 w:=httptest.NewRecorder()
 a.audioStatusHandler(w,r)
 if w.Code!=200{t.Fatalf("status code %d",w.Code)}
 var out map[string]interface{}
 if err:=json.Unmarshal(w.Body.Bytes(),&out);err!=nil{t.Fatal(err)}
 if out["proAudioEntitlementVerified"]==true||out["microphoneRoutingReady"]==true {
   t.Fatal("status claims unavailable Pro or microphone features are enabled")
 }
}
