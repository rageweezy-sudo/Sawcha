package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"time"
)

func main() {
	if runtime.GOOS != "windows" {
		fmt.Fprintln(os.Stderr, "SAWCHA Latency Lab is a Windows 10/11 diagnostic companion. Run the Windows build.")
		os.Exit(1)
	}
	token, err := randomToken()
	if err != nil {
		log.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	app := &server{token: token, origin: "http://" + listener.Addr().String(), client: &http.Client{Timeout: 55 * time.Second}}
	srv := &http.Server{Handler: app.routes(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 40 * time.Second}
	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Println(err)
		}
	}()
	address := app.origin
	fmt.Println("SAWCHA Latency Lab local interface:", address)
	// Browser UI stays on localhost; no firewall rules or inbound exposure.
	_ = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", address).Start()
	// Graceful Ctrl+C lets in-flight bounded ETW captures clean up.
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	<-signalCtx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
