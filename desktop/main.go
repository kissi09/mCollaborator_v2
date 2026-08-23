// Command mCollaborator is the desktop shell around the mCollaborator server.
//
// It opens a native window - WebView2 on Windows, the system webview elsewhere -
// rather than a browser tab, so the app has its own icon, its own taskbar entry
// and no address bar. The web application inside it is unchanged: the shell
// starts the real server binary on a private loopback port and proxies the
// window's requests to it.
//
// Nothing in ../backend is modified or imported by this program. That is the
// point of the arrangement: the server that ships inside the desktop app is
// byte-for-byte the server that ships on its own.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// Window geometry. The report wizard is the widest thing in the app and wants
// room; the minimum is the point below which its step navigation starts to wrap.
const (
	windowWidth     = 1440
	windowHeight    = 900
	windowMinWidth  = 1024
	windowMinHeight = 700
)

func main() {
	logTo, err := openShellLog()
	if err == nil {
		defer logTo.Close()
		log.SetOutput(logTo)
	}
	log.SetFlags(log.LstdFlags)
	log.Printf("mCollaborator desktop starting")

	backend, err := startServer(context.Background())
	if err != nil {
		// There is no console behind a GUI build, so the failure has to be put
		// somewhere the user will actually find it.
		fatal("mCollaborator could not start", err)
	}
	defer backend.Stop()
	log.Printf("server ready on %s", backend.baseURL)

	proxy, err := newProxy(backend.baseURL)
	if err != nil {
		backend.Stop()
		fatal("mCollaborator could not start", err)
	}

	err = wails.Run(&options.App{
		Title:     "mCollaborator",
		Width:     windowWidth,
		Height:    windowHeight,
		MinWidth:  windowMinWidth,
		MinHeight: windowMinHeight,

		// Everything the window asks for is forwarded to the server process.
		AssetServer: &assetserver.Options{Handler: proxy},

		// Two copies would open two servers against one SQLite file. The second
		// launch raises the existing window instead.
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "com.cyberteq.mcollaborator",
		},

		BackgroundColour: &options.RGBA{R: 11, G: 13, B: 20, A: 1},

		OnShutdown: func(ctx context.Context) {
			log.Printf("window closed, stopping server")
			backend.Stop()
		},

		Windows: &windows.Options{
			// The web app draws its own light and dark themes; the webview must
			// not tint them.
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			// A report can be a hundred pages of DOCX preview - let the user
			// zoom without the shell fighting them.
			ZoomFactor:           1.0,
			DisableWindowIcon:    false,
			WebviewUserDataPath:  webviewDataPath(),
			WebviewBrowserPath:   "",
			DisablePinchZoom:     false,
			IsZoomControlEnabled: true,
		},
	})
	if err != nil {
		log.Printf("wails exited with error: %v", err)
		fatal("mCollaborator stopped unexpectedly", err)
	}
	log.Printf("mCollaborator desktop exited cleanly")
}

// newProxy forwards every window request to the server process.
//
// The window's own origin is wails://wails.localhost, so the app's relative
// fetches to /api/v1/... arrive here and are passed straight through. Keeping a
// single origin also keeps the session token in one localStorage bucket across
// restarts.
func newProxy(baseURL string) (http.Handler, error) {
	target, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse server address %q: %w", baseURL, err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("proxy error for %s: %v", r.URL.Path, err)
		http.Error(w, "The mCollaborator server is not responding.", http.StatusBadGateway)
	}
	return proxy, nil
}

// webviewDataPath keeps WebView2's cache with the app's own data rather than
// letting it default into the install directory.
func webviewDataPath() string {
	dir, err := dataDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(dir, "webview")
	if err := os.MkdirAll(path, 0o755); err != nil {
		return ""
	}
	return path
}

// openShellLog puts the shell's own diagnostics beside the server's, so a
// failed launch leaves a trail even though there is no console.
func openShellLog() (*os.File, error) {
	dir, err := dataDir()
	if err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(dir, "mCollaborator-desktop.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}
