package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// releaseServer stands in for the GitHub releases API: it answers /latest with one
// release whose asset for assetName is served from /download.
func releaseServer(t *testing.T, assetName, binary string, assetStatus int) string {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v1.2.3","assets":[
			{"name":"skillset_plan9_mips","browser_download_url":%q},
			{"name":%q,"browser_download_url":%q}]}`,
			srv.URL+"/wrong", assetName, srv.URL+"/download")
	})
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		if assetStatus != http.StatusOK {
			w.WriteHeader(assetStatus)
			return
		}
		fmt.Fprint(w, binary)
	})
	return srv.URL + "/latest"
}

// installed writes a stand-in for the running binary and returns its path.
func installed(t *testing.T, body string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "skillset")
	if err := os.WriteFile(exe, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func read(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func thisPlatform() string { return "skillset_" + runtime.GOOS + "_" + runtime.GOARCH }

func TestUpdateReplacesTheBinaryWithTheLatestBuildForThisPlatform(t *testing.T) {
	api := releaseServer(t, thisPlatform(), "new binary", http.StatusOK)
	exe := installed(t, "old binary")
	out := &bytes.Buffer{}

	if err := update(api, exe, out); err != nil {
		t.Fatal(err)
	}

	if got := read(t, exe); got != "new binary" {
		t.Errorf("binary is %q, want the downloaded one", got)
	}
	if !strings.Contains(out.String(), "v1.2.3") {
		t.Errorf("output %q does not name the release", out.String())
	}
	info, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Errorf("binary is not executable: mode %v", info.Mode())
	}
}

func TestUpdateReportsAReleaseWithNoBuildForThisPlatform(t *testing.T) {
	api := releaseServer(t, "skillset_solaris_sparc", "new binary", http.StatusOK)
	exe := installed(t, "old binary")

	err := update(api, exe, &bytes.Buffer{})

	if err == nil || !strings.Contains(err.Error(), runtime.GOOS) {
		t.Fatalf("error is %v, want one naming this platform", err)
	}
	if got := read(t, exe); got != "old binary" {
		t.Errorf("binary is %q, want it untouched", got)
	}
}

func TestUpdateKeepsTheBinaryWhenTheDownloadFails(t *testing.T) {
	api := releaseServer(t, thisPlatform(), "", http.StatusInternalServerError)
	exe := installed(t, "old binary")

	if err := update(api, exe, &bytes.Buffer{}); err == nil {
		t.Fatal("a failed download reported success")
	}

	if got := read(t, exe); got != "old binary" {
		t.Errorf("binary is %q, want it untouched", got)
	}
	left, err := os.ReadDir(filepath.Dir(exe))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 {
		t.Errorf("update left %d files beside the binary, want only the binary", len(left))
	}
}

func TestUpdateReportsAnUnreachableReleaseAPI(t *testing.T) {
	api := releaseServer(t, thisPlatform(), "new binary", http.StatusOK)
	exe := installed(t, "old binary")

	if err := update(strings.TrimSuffix(api, "/latest")+"/missing", exe, &bytes.Buffer{}); err == nil {
		t.Fatal("a 404 from the release API reported success")
	}
	if got := read(t, exe); got != "old binary" {
		t.Errorf("binary is %q, want it untouched", got)
	}
}
