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

func thisPlatform() string { return "skillset_" + runtime.GOOS + "_" + runtime.GOARCH }

// running pretends the binary was stamped with this version by the linker.
func running(t *testing.T, v string) {
	t.Helper()
	was := version
	version = v
	t.Cleanup(func() { version = was })
}

func TestUpdateReplacesTheBinaryWithTheLatestBuildForThisPlatform(t *testing.T) {
	running(t, "v1.0.0")
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

func TestUpdateDownloadsNothingWhenTheLatestReleaseIsAlreadyInstalled(t *testing.T) {
	for _, v := range []string{"v1.2.3", "1.2.3"} {
		t.Run(v, func(t *testing.T) {
			running(t, v)
			api := releaseServer(t, thisPlatform(), "new binary", http.StatusOK)
			exe := installed(t, "old binary")
			out := &bytes.Buffer{}

			if err := update(api, exe, out); err != nil {
				t.Fatal(err)
			}

			if got := read(t, exe); got != "old binary" {
				t.Errorf("binary is %q, want it untouched", got)
			}
			if !strings.Contains(out.String(), "already the latest") {
				t.Errorf("output %q does not say the binary is current", out.String())
			}
		})
	}
}

func TestUpdateReplacesABuildThatWasNeverReleased(t *testing.T) {
	running(t, "dev (abc1234)")
	api := releaseServer(t, thisPlatform(), "new binary", http.StatusOK)
	exe := installed(t, "old binary")
	out := &bytes.Buffer{}

	if err := update(api, exe, out); err != nil {
		t.Fatal(err)
	}

	if got := read(t, exe); got != "new binary" {
		t.Errorf("binary is %q, want the downloaded one", got)
	}
	if !strings.Contains(out.String(), "not a release") {
		t.Errorf("output %q does not warn that this build is not a release", out.String())
	}
}

func TestBinaryVersionDescribesABuildTheLinkerDidNotStamp(t *testing.T) {
	running(t, "")

	if got := binaryVersion(); !strings.HasPrefix(got, "dev") {
		t.Errorf("version of an unstamped build is %q, want it to own up to being a dev build", got)
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
	running(t, "v1.0.0")
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

// downloadServer serves body at its root, chunked when sendLength is false so the client
// sees no Content-Length.
func downloadServer(t *testing.T, body string, sendLength bool) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Go only works out a Content-Length for itself on bodies small enough to buffer.
		if sendLength {
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestReplaceBinaryShowsHowMuchOfTheDownloadHasArrived(t *testing.T) {
	url := downloadServer(t, strings.Repeat("x", 3<<20), true)
	exe := installed(t, "old binary")
	progress := &bytes.Buffer{}

	if err := replaceBinary(exe, url, progress); err != nil {
		t.Fatal(err)
	}

	got := progress.String()
	if !strings.HasPrefix(got, "\r") || !strings.HasSuffix(got, "\n") {
		t.Errorf("progress %q does not overwrite one line and then close it", got)
	}
	if !strings.Contains(got, "100%") || !strings.Contains(got, "3.0 MB") {
		t.Errorf("progress %q does not reach 100%% of 3.0 MB", got)
	}
}

func TestReplaceBinaryCountsBytesWhenTheServerSendsNoLength(t *testing.T) {
	url := downloadServer(t, strings.Repeat("x", 3<<20), false)
	exe := installed(t, "old binary")
	progress := &bytes.Buffer{}

	if err := replaceBinary(exe, url, progress); err != nil {
		t.Fatal(err)
	}

	got := progress.String()
	if !strings.Contains(got, "3.0 MB") {
		t.Errorf("progress %q does not count the bytes that arrived", got)
	}
	if strings.Contains(got, "%") {
		t.Errorf("progress %q claims a percentage with no total to take it of", got)
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
