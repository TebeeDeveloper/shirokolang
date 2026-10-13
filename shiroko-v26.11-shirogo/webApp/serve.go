//go:build !js && !wasm

package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

//go:embed static
var staticFS embed.FS

func init() {
	_ = mime.AddExtensionType(".wasm", "application/wasm")
}

func main() {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://127.0.0.1:%d", port)

	go func() {
		time.Sleep(150 * time.Millisecond)
		if err := openBrowser(url); err != nil {
			log.Printf("Không mở được browser: %v", err)
			log.Printf("Vào thủ công: %s", url)
		}
	}()

	log.Printf("🟢 ShiroWeb đang chạy tại %s", url)
	log.Printf("   Đóng cửa sổ này để tắt.")

	if err := http.Serve(listener, http.FileServer(http.FS(sub))); err != nil {
		log.Fatal(err)
	}
}

func openBrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd, args = "open", []string{url}
	case "linux":
		cmd, args = "xdg-open", []string{url}
	default:
		return fmt.Errorf("OS không hỗ trợ: %s", runtime.GOOS)
	}
	return exec.Command(cmd, args...).Start()
}
