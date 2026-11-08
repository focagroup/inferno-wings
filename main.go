// Command inferno-wings is a tiny launcher for the Inferno Wings browser game.
//
// The whole game (game/) is embedded into the binary. On start it serves the
// game on 127.0.0.1 and opens it in a borderless app window using an installed
// Chromium-based browser (Chrome, Edge, Chromium, Brave, Vivaldi). If none is
// found it falls back to the system default browser. The launcher exits when
// the window is closed or the page stops sending heartbeats.
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

//go:embed all:game
var gameFiles embed.FS

// A fixed port keeps the page origin stable, so best scores saved in
// localStorage survive restarts. Falls back to a random port if taken.
const preferredPort = 47823

// Injected into the served page: lets the launcher notice when the game is closed.
const heartbeatJS = `<script>(()=>{const ping=()=>fetch('/__alive',{cache:'no-store'}).catch(()=>{});` +
	`ping();setInterval(ping,2000);addEventListener('pagehide',()=>navigator.sendBeacon('/__bye'));})();</script>`

func main() {
	noWindow := flag.Bool("no-window", false, "only run the local server; open the printed URL yourself")
	useDefault := flag.Bool("browser", false, "open in the default browser instead of an app window")
	port := flag.Int("port", preferredPort, "local port to serve the game on")
	flag.Parse()

	root, err := fs.Sub(gameFiles, "game")
	if err != nil {
		fatal(err)
	}
	index, err := fs.ReadFile(root, "index.html")
	if err != nil {
		fatal(err)
	}
	page := strings.Replace(string(index), "</body>", heartbeatJS+"</body>", 1)

	var lastPing, lastBye atomic.Int64
	static := http.FileServer(http.FS(root))
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write([]byte(page))
			return
		}
		static.ServeHTTP(w, r)
	})
	mux.HandleFunc("/__alive", func(w http.ResponseWriter, _ *http.Request) {
		lastPing.Store(time.Now().UnixNano())
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/__bye", func(w http.ResponseWriter, _ *http.Request) {
		lastBye.Store(time.Now().UnixNano())
		w.WriteHeader(http.StatusNoContent)
	})

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		if ln, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
			fatal(err)
		}
	}
	url := fmt.Sprintf("http://%s/", ln.Addr().String())
	go func() { _ = http.Serve(ln, mux) }()
	fmt.Println("Inferno Wings is running at", url)

	if *noWindow {
		fmt.Println("Press Ctrl+C to quit.")
		select {}
	}

	done := make(chan struct{})
	start := time.Now()
	opened := false
	if !*useDefault {
		if cmd := appWindow(url); cmd != nil && cmd.Start() == nil {
			opened = true
			go func() {
				_ = cmd.Wait()
				// A browser that was already running may hand the window to its
				// existing process and return at once; then the heartbeat decides.
				if time.Since(start) > 4*time.Second {
					closeOnce(done)
				}
			}()
		}
	}
	if !opened {
		if err := openDefault(url); err != nil {
			fmt.Println("Could not open a browser. Open this URL yourself:", url)
		}
	}

	// Watchdog: quit once the page is gone.
	go func() {
		for range time.Tick(time.Second) {
			ping, bye := lastPing.Load(), lastBye.Load()
			switch {
			case ping == 0 && time.Since(start) > 3*time.Minute: // never loaded
				closeOnce(done)
			case bye > ping && time.Since(time.Unix(0, bye)) > 5*time.Second: // closed, not reloaded
				closeOnce(done)
			case ping != 0 && time.Since(time.Unix(0, ping)) > 150*time.Second: // background tabs ping rarely
				closeOnce(done)
			}
		}
	}()
	<-done
}

var closed atomic.Bool

func closeOnce(ch chan struct{}) {
	if closed.CompareAndSwap(false, true) {
		close(ch)
	}
}

// appWindow returns a command that opens url as a standalone app window in a
// Chromium-based browser, or nil if none is installed.
func appWindow(url string) *exec.Cmd {
	browser := findBrowser()
	if browser == "" {
		return nil
	}
	profile := filepath.Join(os.TempDir(), "inferno-wings-profile")
	if dir, err := os.UserConfigDir(); err == nil {
		profile = filepath.Join(dir, "inferno-wings", "browser-profile")
	}
	_ = os.MkdirAll(profile, 0o755)
	return exec.Command(browser,
		"--app="+url,
		"--user-data-dir="+profile,
		"--no-first-run",
		"--no-default-browser-check",
		"--window-size=1280,800",
		"--autoplay-policy=no-user-gesture-required",
		"--class=inferno-wings",
	)
}

func findBrowser() string {
	var paths, names []string
	switch runtime.GOOS {
	case "windows":
		for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LocalAppData"} {
			base := os.Getenv(env)
			if base == "" {
				continue
			}
			paths = append(paths,
				filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"),
				filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(base, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
				filepath.Join(base, "Chromium", "Application", "chrome.exe"),
			)
		}
		names = []string{"msedge", "chrome"}
	case "darwin":
		paths = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	default:
		names = []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
			"brave-browser", "brave", "microsoft-edge", "microsoft-edge-stable", "vivaldi", "vivaldi-stable"}
	}
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

func openDefault(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "inferno-wings:", err)
	os.Exit(1)
}
