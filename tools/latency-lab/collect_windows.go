//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// This script reads a curated subset of Windows CIM objects. It intentionally
// EXCLUDES user names, machine names, GUIDs, MAC/IP, serials, paths, command
// lines, downloaded content and event-message bodies. Deep mode is opt-in.
const psSnapshot = `
$ErrorActionPreference = 'SilentlyContinue'
$ProgressPreference = 'SilentlyContinue'
function items($value) { if($null -eq $value){ return @() }; return @($value) }
function safeget($class) { try { @(Get-CimInstance -ClassName $class -ErrorAction Stop) } catch { @() } }
$cpu = @(safeget 'Win32_Processor' | Select-Object Name,Manufacturer,NumberOfCores,NumberOfLogicalProcessors,MaxClockSpeed,CurrentClockSpeed,LoadPercentage)
$os = @(safeget 'Win32_OperatingSystem' | Select-Object Caption,Version,BuildNumber,OSArchitecture,LastBootUpTime,TotalVisibleMemorySize,FreePhysicalMemory)
$ram = @(safeget 'Win32_PhysicalMemory' | Select-Object Capacity,Speed,ConfiguredClockSpeed,MemoryType,FormFactor,Manufacturer)
$gpu = @(safeget 'Win32_VideoController' | Select-Object Name,AdapterRAM,DriverVersion,VideoProcessor,CurrentHorizontalResolution,CurrentVerticalResolution,CurrentRefreshRate)
$disk = @(safeget 'Win32_DiskDrive' | Select-Object Model,MediaType,InterfaceType,Size,Status)
$volume = @(safeget 'Win32_LogicalDisk' | Where-Object DriveType -eq 3 | Select-Object DeviceID,Size,FreeSpace,FileSystem)
$audio = @(safeget 'Win32_SoundDevice' | Select-Object Name,Manufacturer,Status)
$network = @(safeget 'Win32_NetworkAdapter' | Where-Object { $_.PhysicalAdapter -and $_.NetEnabled } | Select-Object Name,AdapterType,Speed,NetConnectionStatus)
$board = @(safeget 'Win32_BaseBoard' | Select-Object Manufacturer,Product,Version)
$bios = @(safeget 'Win32_BIOS' | Select-Object Manufacturer,SMBIOSBIOSVersion,ReleaseDate)
$usb = @(safeget 'Win32_USBController' | Select-Object Name,Status)
$power = ''; try{$power = (powercfg.exe /getactivescheme | Out-String).Trim() -replace '[0-9A-Fa-f-]{36}', '[GUID REMOVED]'}catch{}
$processes = @(); try {$processes = @(Get-Process | Sort-Object WorkingSet64 -Descending | Select-Object -First 18 ProcessName,CPU,WorkingSet64,Handles,@{Name='ThreadCount';Expression={$_.Threads.Count}})}catch{}
$sawcha = @(); try {$sawcha = @(Get-Process | Where-Object {$_.ProcessName -like 'SAWCHA*'} | Select-Object ProcessName,CPU,WorkingSet64,Handles,@{Name='ThreadCount';Expression={$_.Threads.Count}})}catch{}
$memory = @(safeget 'Win32_PerfFormattedData_PerfOS_Memory' | Select-Object AvailableMBytes,PagesPerSec,PageReadsPerSec,PageWritesPerSec,PoolNonpagedBytes)
$perfCPU = @(safeget 'Win32_PerfFormattedData_PerfOS_Processor' | Where-Object Name -eq '_Total' | Select-Object PercentProcessorTime,PercentPrivilegedTime,PercentDPCTime,PercentInterruptTime,DPCsQueuedPerSec,InterruptsPerSec)
$perfDisk = @(safeget 'Win32_PerfFormattedData_PerfDisk_PhysicalDisk' | Where-Object Name -eq '_Total' | Select-Object AvgDisksecPerRead,AvgDisksecPerWrite,AvgDiskQueueLength,PercentDiskTime,DiskReadsPerSec,DiskWritesPerSec)
$report=[ordered]@{
 cpu=if($cpu.Count){$cpu[0]}else{$null}; os=if($os.Count){$os[0]}else{$null}; memoryModules=@($ram); gpu=@($gpu); physicalDisks=@($disk); logicalVolumes=@($volume);
 audioDevices=@($audio); networkAdapters=@($network); baseBoard=if($board.Count){$board[0]}else{$null}; bios=if($bios.Count){$bios[0]}else{$null}; usbControllers=@($usb);
 powerPlan=$power; topProcesses=@($processes); sawchaProcesses=@($sawcha); memoryCounters=if($memory.Count){$memory[0]}else{$null}; cpuCounters=if($perfCPU.Count){$perfCPU[0]}else{$null}; diskCounters=if($perfDisk.Count){$perfDisk[0]}else{$null}
}
if('%DEEP%' -eq 'true') {
 $drivers = @(); try {
 $drivers = @(Get-CimInstance Win32_PnPSignedDriver | Where-Object { $_.DeviceClass -in @('DISPLAY','MEDIA','NET','SYSTEM','USB','HDC','SCSIADAPTER','HIDCLASS','MOUSE','KEYBOARD') } | Select-Object -First 220 DeviceName,DeviceClass,Manufacturer,DriverVersion,DriverDate,IsSigned,InfName)
 }catch{}
 $events = @(); try {
 $events = @(Get-WinEvent -FilterHashtable @{LogName='System';Level=1,2;StartTime=(Get-Date).AddDays(-2)} -MaxEvents 40 | Select-Object TimeCreated,Id,ProviderName,LevelDisplayName)
 }catch{}
 $services = @(); try{$services = @(Get-Service | Group-Object Status | Select-Object Name,Count)}catch{}
 $report['drivers']=$drivers; $report['recentSystemErrorIDs']=$events; $report['servicesByStatus']=$services
}
$report | ConvertTo-Json -Depth 6 -Compress
`

func windowsSnapshot(parent context.Context, deep bool) (map[string]any, error) {
	wait := 15 * time.Second
	if deep {
		wait = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, wait)
	defer cancel()
	script := strings.ReplaceAll(psSnapshot, "%DEEP%", "false")
	if deep {
		script = strings.ReplaceAll(psSnapshot, "%DEEP%", "true")
	}
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script)
	// Only this fixed, embedded script executes. No user-controlled shell input.
	b, err := cmd.Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("Windows CIM scan timed out; try standard scan or check WMI health")
		}
		return nil, fmt.Errorf("Windows CIM scan failed: %v", err)
	}
	result := map[string]any{}
	// Windows PowerShell can emit a UTF-8 BOM or prefix warnings; reject invalid JSON.
	clean := strings.TrimSpace(strings.TrimPrefix(string(b), "\ufeff"))
	if len(clean) > 4<<20 {
		return nil, fmt.Errorf("Windows returned an unusually large diagnostic response")
	}
	if err := json.Unmarshal([]byte(clean), &result); err != nil {
		return nil, fmt.Errorf("Windows returned invalid structured diagnostic data: %w", err)
	}
	return result, nil
}
