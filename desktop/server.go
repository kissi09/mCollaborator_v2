package main

// Supervision of the mCollaborator server process.
//
// The desktop app deliberately does not contain the server. The backend is its
// own Go module and its own `package main`, and it stays that way: wrapping it
// must not require reshaping it. So the window runs the real server binary as a
// child process on a private port and proxies to it, which means the desktop
// build and the plain server build are the same program, tested the same way.

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// serverBinaryNames are the names the backend may have been built under,
// newest first. The installer ships it as mCollaborator-server.exe so it is
// obvious which of the two executables in the install directory is which.
var serverBinaryNames = []string{
	"mCollaborator-server.exe",
	"mCollaborator.exe",
}

// devServerDirs are checked when the app is run from a source checkout rather
// than an install, so `wails dev` works without staging a build first.
var devServerDirs = []string{
	filepath.Join("..", "backend"),
	filepath.Join("..", "..", "backend"),
}

// server is the supervised backend process.
type server struct {
	cmd     *exec.Cmd
	baseURL string
	logFile *os.File
}

// locateServerBinary finds the backend executable: first beside this one, which
// is how it is installed, then in the sibling source tree, which is how it is
// developed.
func locateServerBinary() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate own executable: %w", err)
	}
	dir := filepath.Dir(self)

	var tried []string
	for _, name := range serverBinaryNames {
		candidate := filepath.Join(dir, name)
		tried = append(tried, candidate)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			// Guard against the app finding itself when both are called
			// mCollaborator.exe.
			if !sameFile(candidate, self) {
				return candidate, nil
			}
		}
	}
	for _, rel := range devServerDirs {
		for _, name := range serverBinaryNames {
			candidate := filepath.Join(dir, rel, name)
			tried = append(tried, candidate)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				if abs, err := filepath.Abs(candidate); err == nil {
					return abs, nil
				}
			}
		}
	}
	return "", fmt.Errorf("could not find the mCollaborator server executable; looked in:\n  %s",
		joinLines(tried))
}

func sameFile(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

func joinLines(items []string) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += "\n  "
		}
		out += s
	}
	return out
}

// freePort asks the OS for an unused loopback port and hands back the number.
//
// There is an unavoidable gap between closing this listener and the server
// binding it. Nothing else on a desktop machine is hunting for ports, and the
// alternative - a fixed port - collides with anything already there, including
// a second copy of the server the user started by hand.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// dataDir is where the server keeps its database and generated reports.
//
// It is a per-user directory, never the install directory: Program Files is not
// writable by a standard user, and engagement data has no business living
// beside the binaries anyway.
func dataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "mCollaborator")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// startServer launches the backend and waits for it to answer /health.
func startServer(ctx context.Context) (*server, error) {
	bin, err := locateServerBinary()
	if err != nil {
		return nil, err
	}
	dir, err := dataDir()
	if err != nil {
		return nil, fmt.Errorf("prepare data directory: %w", err)
	}
	port, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("reserve a port: %w", err)
	}

	// The server writes its database and reports relative to the working
	// directory, so the working directory is what places them per-user.
	logPath := filepath.Join(dir, "mCollaborator-server.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open server log: %w", err)
	}

	cmd := exec.Command(bin)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PORT=%d", port),
		fmt.Sprintf("MCOLLABORATOR_DB_PATH=%s", filepath.Join(dir, "data", "mcollaborator.db")),
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	hideConsoleWindow(cmd)

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("start %s: %w", bin, err)
	}

	s := &server{cmd: cmd, baseURL: fmt.Sprintf("http://127.0.0.1:%d", port), logFile: logFile}
	if err := s.waitReady(ctx); err != nil {
		s.Stop()
		return nil, err
	}
	return s, nil
}

// serverStartTimeout is generous because a cold start on a slow disk also has
// to open and migrate the SQLite database.
const serverStartTimeout = 30 * time.Second

// waitReady polls /health until the server answers or gives up. It also watches
// for the process dying, so a server that exits immediately reports that rather
// than making the user wait out the whole timeout.
func (s *server) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(serverStartTimeout)
	client := &http.Client{Timeout: 2 * time.Second}
	exited := make(chan error, 1)
	go func() { exited <- s.cmd.Wait() }()

	for {
		select {
		case err := <-exited:
			return fmt.Errorf("the server exited during startup (%v); see %s",
				err, s.logFile.Name())
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resp, err := client.Get(s.baseURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the server did not answer on %s within %s; see %s",
				s.baseURL, serverStartTimeout, s.logFile.Name())
		}
		time.Sleep(150 * time.Millisecond)
	}
}

// Stop ends the server process. The window closing is the only way out of the
// app, so this is what stops the machine being left with an orphaned server
// holding the database open.
func (s *server) Stop() {
	if s == nil {
		return
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	if s.logFile != nil {
		_ = s.logFile.Close()
	}
}
