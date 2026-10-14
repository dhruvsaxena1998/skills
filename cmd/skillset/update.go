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
	"strings"
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
	if err := replaceBinary(exe, url); err != nil {
		return err
	}
	fmt.Fprintf(out, "skillset %s installed in %s\n", tag, exe)
	return nil
}

// latestRelease returns the newest tag and the download URL of its build for this platform.
func latestRelease(api string) (tag, url string, err error) {
	body, err := get(api)
	if err != nil {
		return "", "", err
	}
	defer body.Close()
	var rel struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(body).Decode(&rel); err != nil {
		return "", "", fmt.Errorf("read the latest release: %w", err)
	}
	// Match on the platform, not the whole file name, which carries .exe on Windows.
	platform := runtime.GOOS + "_" + runtime.GOARCH
	for _, a := range rel.Assets {
		if strings.Contains(a.Name, platform) {
			return rel.TagName, a.URL, nil
		}
	}
	return "", "", fmt.Errorf("release %s has no build for %s", rel.TagName, platform)
}

func get(url string) (io.ReadCloser, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp.Body, nil
}

// replaceBinary downloads url over exe, through a temp file beside it so that a download
// cut off halfway never leaves a broken binary on the PATH.
func replaceBinary(exe, url string) error {
	body, err := get(url)
	if err != nil {
		return err
	}
	defer body.Close()
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
