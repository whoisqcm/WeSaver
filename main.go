package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"wesaver/internal/ui"

	"github.com/jchv/go-webview2"
)

func main() {
	server := ui.NewServer()

	url, err := server.StartInBackground()
	if err != nil {
		fmt.Fprintf(os.Stderr, "启动失败: %v\n", err)
		os.Exit(1)
	}
	defer server.Shutdown()

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "WeSaver",
			Width:  1150,
			Height: 750,
			Center: true,
		},
	})

	if w == nil {
		// WebView2 runtime not available — fallback to browser
		fmt.Printf("WeSaver 已启动: %s\n", url)
		fmt.Println("WebView2 不可用，已在浏览器中打开。按 Ctrl+C 退出。")
		go ui.OpenBrowser(url)

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		return
	}
	defer w.Destroy()

	w.Navigate(url)
	w.Run()
}
