package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// latestReleaseAPI is the public endpoint: no token, no auth, 60 calls an hour per IP.
const latestReleaseAPI = "https://api.github.com/repos/dhruvsaxena1998/skills/releases/latest"

var httpClient = &http.Client{Timeout: 5 * time.Minute}

// updateCmd replaces the running binary with the latest release build for this platform.
// shortcut: it downloads every time, even when the binary is already current; stamp the
// tag into the build with -ldflags and compare first if that download starts to hurt.
func updateCmd(args []string, out io.Writer) error {
	if len(args) > 0 {
		return errors.New("usage: skillset update")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// Follow the link so an update replaces the binary itself, not a symlink to it.
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return update(latestReleaseAPI, exe, out)
}

func update(api, exe string, out io.Writer) error {
	tag, url, err := latestRelease(api)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "downloading skillset %s from %s\n", tag, url)
	// A meter redrawing itself with \r is noise in a log file, so only draw one on a terminal.
	var progress io.Writer
	if info, err := os.Stderr.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
		progress = os.Stderr
	}
	if err := replaceBinary(exe, url, progress); err != nil {
		return err
	}
	fmt.Fprintf(out, "skillset %s installed in %s\n", tag, exe)
	return nil
}

// latestRelease returns the newest tag and the download URL of its build for this platform.
func latestRelease(api string) (tag, url string, err error) {
	resp, err := get(api)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var rel struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", "", fmt.Errorf("read the latest release: %w", err)
	}
	// The exact names .goreleaser.yaml produces, so a checksum or signature file for
	// the same platform can never be downloaded over the binary.
	platform := "skillset_" + runtime.GOOS + "_" + runtime.GOARCH
	for _, a := range rel.Assets {
		if a.Name == platform || a.Name == platform+".exe" {
			return rel.TagName, a.URL, nil
		}
	}
	return "", "", fmt.Errorf("release %s has no build for %s", rel.TagName, platform)
}

func get(url string) (*http.Response, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp, nil
}

// meter prints how much of a download has arrived, overwriting one line as bytes pass
// through it. It is an io.Writer so a TeeReader can feed it the copy.
type meter struct {
	w     io.Writer
	total int64 // -1 when the server sent no Content-Length
	done  int64
	shown int64 // last step drawn, so a 14 MB download does not print a thousand lines
}

func (m *meter) Write(b []byte) (int, error) {
	m.done += int64(len(b))
	// One line per percent, or per megabyte when there is no total to take a percent of.
	step := m.done >> 20
	if m.total > 0 {
		step = m.done * 100 / m.total
	}
	if step != m.shown {
		m.shown = step
		if m.total > 0 {
			fmt.Fprintf(m.w, "\r  %3d%%  %.1f of %.1f MB", step, megabytes(m.done), megabytes(m.total))
		} else {
			fmt.Fprintf(m.w, "\r  %.1f MB", megabytes(m.done))
		}
	}
	return len(b), nil
}

func megabytes(n int64) float64 { return float64(n) / (1 << 20) }

// replaceBinary downloads url over exe, through a temp file beside it so that a download
// cut off halfway never leaves a broken binary on the PATH. A non-nil progress gets a
// one-line meter as the bytes arrive.
func replaceBinary(exe, url string, progress io.Writer) error {
	resp, err := get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body := io.Reader(resp.Body)
	if progress != nil {
		body = io.TeeReader(body, &meter{w: progress, total: resp.ContentLength, shown: -1})
		defer fmt.Fprintln(progress) // close the line the meter has been overwriting
	}
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".skillset-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // a no-op once the rename below has moved it
	if _, err := io.Copy(tmp, body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		// Windows refuses to overwrite a running .exe, but it does let us rename one.
		// shortcut: the displaced .old file stays until the next update removes it,
		// because this process is still running from it; delete it by hand to be rid of it.
		old := exe + ".old"
		os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			return err
		}
		if err := os.Rename(tmp.Name(), exe); err != nil {
			os.Rename(old, exe)
			return err
		}
		return nil
	}
	return os.Rename(tmp.Name(), exe)
}
