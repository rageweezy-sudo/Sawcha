//go:build !windows

package main

func runningProcesses() (map[string]bool, error) { return map[string]bool{}, nil }
