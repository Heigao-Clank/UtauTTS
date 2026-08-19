package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var logPath = filepath.Join(os.TempDir(), "utautts-updater.log")

func logf(format string, args ...any) {
	line := fmt.Sprintf("[%s] %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
	if file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		_, _ = file.WriteString(line)
		_ = file.Close()
	}
	fmt.Print(line)
}

func main() {
	target := flag.String("target", "", "package root directory to update")
	downloadURL := flag.String("url", "", "zip download URL")
	pid := flag.Int("pid", 0, "PID of the running GUI to wait for before replacing files")
	version := flag.String("version", "", "incoming release tag (diagnostics)")
	preserveFlag := flag.String("preserve", "voice", "comma-separated relative paths kept from the old install")
	flag.Parse()

	if *target == "" || *downloadURL == "" {
		logf("usage: utautts-updater -target <dir> -url <zip-url> [-pid <pid>] [-version <tag>]")
		os.Exit(2)
	}
	var preserve []string
	for _, part := range strings.Split(*preserveFlag, ",") {
		if part = strings.TrimSpace(part); part != "" {
			preserve = append(preserve, part)
		}
	}

	ok := true
	if err := run(*target, *downloadURL, *pid, *version, preserve); err != nil {
		ok = false
		logf("update failed: %v", err)
	}
	// Always relaunch: on success the new build, on failure the untouched old
	// build, so a failed update never strands the user without an app.
	launchApp(*target)
	if !ok {
		os.Exit(1)
	}
}

func run(target, url string, pid int, version string, preserve []string) error {
	logf("utautts-updater start: target=%s version=%s", target, version)
	absolute, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	target = absolute

	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		return fmt.Errorf("target directory is not a directory: %s", target)
	}

	if !waitForExit(pid, 5*time.Minute) {
		logf("parent process %d did not exit within timeout; continuing anyway", pid)
	}

	stage := target + ".stage"
	old := target + ".old"

	zipPath := filepath.Join(os.TempDir(), "utautts-update-"+sanitizeToken(version)+".zip")
	if err := download(url, zipPath); err != nil {
		return err
	}

	if err := os.RemoveAll(stage); err != nil {
		return err
	}
	if err := extractZip(zipPath, stage); err != nil {
		return err
	}
	if err := os.Remove(zipPath); err != nil {
		logf("removing downloaded archive failed: %v", err)
	}
	if err := normalizeStage(stage); err != nil {
		return err
	}

	for _, rel := range preserve {
		if err := preservePath(target, stage, rel); err != nil {
			logf("preserve %s: %v", rel, err)
		}
	}

	if err := os.RemoveAll(old); err != nil {
		return err
	}
	if err := os.Rename(target, old); err != nil {
		return fmt.Errorf("move current install aside: %w", err)
	}
	if err := os.Rename(stage, target); err != nil {
		_ = os.Rename(old, target)
		return fmt.Errorf("move new install into place: %w", err)
	}
	if err := os.RemoveAll(old); err != nil {
		logf("removing old install backup failed (left at %s): %v", old, err)
	}
	logf("update applied to %s", target)
	return nil
}

func waitForExit(pid int, timeout time.Duration) bool {
	if pid <= 0 {
		return true
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func download(url, dest string) error {
	logf("downloading %s", url)
	client := &http.Client{Timeout: 20 * time.Minute}
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "UtauTTS-updater")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %s", response.Status)
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	written, err := io.Copy(out, response.Body)
	if err != nil {
		return err
	}
	logf("downloaded %d bytes to %s", written, dest)
	return nil
}

func extractZip(zipPath, dest string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer reader.Close()
	dest = filepath.Clean(dest)
	for _, file := range reader.File {
		path := filepath.Join(dest, file.Name)
		if path != dest && !strings.HasPrefix(path, dest+string(os.PathSeparator)) {
			return fmt.Errorf("zip entry escapes destination: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		in, err := file.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			_ = in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		_ = in.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func normalizeStage(stage string) error {
	if packageLooksValid(stage) {
		return nil
	}
	entries, err := os.ReadDir(stage)
	if err != nil {
		return err
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		return fmt.Errorf("stage does not look like a UtauTTS package: %s", stage)
	}
	inner := filepath.Join(stage, entries[0].Name())
	if !packageLooksValid(inner) {
		return fmt.Errorf("stage does not look like a UtauTTS package: %s", stage)
	}
	innerEntries, err := os.ReadDir(inner)
	if err != nil {
		return err
	}
	for _, entry := range innerEntries {
		if err := os.Rename(filepath.Join(inner, entry.Name()), filepath.Join(stage, entry.Name())); err != nil {
			return err
		}
	}
	return os.RemoveAll(inner)
}

func packageLooksValid(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "app", "utautts-gui.exe")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "utautts.exe")); err == nil {
		return true
	}
	return false
}

func preservePath(target, stage, rel string) error {
	source := filepath.Join(target, rel)
	destination := filepath.Join(stage, rel)
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("source not found: %s", source)
	}
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	return copyTree(source, destination)
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target, 0o644)
	})
}

func copyFile(source, destination string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func sanitizeToken(value string) string {
	if value == "" {
		return "latest"
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, value)
}

func launchApp(target string) {
	launcher := filepath.Join(target, "utautts.exe")
	if _, err := os.Stat(launcher); err != nil {
		launcher = filepath.Join(target, "app", "utautts-gui.exe")
	}
	if _, err := os.Stat(launcher); err != nil {
		logf("cannot relaunch: %s not found", launcher)
		return
	}
	command := exec.Command(launcher)
	command.Dir = target
	if err := command.Start(); err != nil {
		logf("relaunch failed: %v", err)
		return
	}
	logf("relaunched %s", launcher)
}
