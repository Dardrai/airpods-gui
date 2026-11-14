package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	for tag, want := range map[string]Version{
		"v0.2.1":                       {0, 2, 1, "", false},
		"v0.2.1-beta":                  {0, 2, 1, "beta", false},
		"0.10.0-rc1":                   {0, 10, 0, "rc1", false},
		"v0.2.1-beta-3-gabc1234":       {0, 2, 1, "beta", true},
		"v0.2.1-beta-3-gabc1234-dirty": {0, 2, 1, "beta", true},
		"v0.2.1-beta-dirty":            {0, 2, 1, "beta", true},
		"v0.1.66-26c9206e6934":         {0, 1, 66, "26c9206e6934", false},
	} {
		got, ok := Parse(tag)
		if !ok || got != want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", tag, got, ok, want)
		}
	}
	for _, bad := range []string{"", "dev", "v1.2", "v1.2.x", "v01.2.3", "v-1.2.3", "bb409d2"} {
		if _, ok := Parse(bad); ok {
			t.Errorf("Parse(%q) accepted", bad)
		}
	}
}

func TestNewer(t *testing.T) {
	for _, tc := range []struct {
		latest, running string
		want            bool
	}{
		{"v0.3.0-beta", "v0.2.1-beta", true},
		{"v0.2.1", "v0.2.1-beta", true},
		{"v0.2.1-rc1", "v0.2.1-beta", true},
		{"v0.2.1-beta", "v0.2.1-alpha", true},
		{"v0.2.1-beta", "v0.2.1-beta", false},
		{"v0.2.1-beta", "v0.2.1-beta-3-gabc1234", false},
		{"v0.2.1-beta", "v0.2.1-beta-dirty", false},
		{"v0.2.1-beta", "v0.2.1", false},
		{"v0.2.0", "v0.2.1-beta", false},
		{"v1.0.0", "v0.99.99", true},
	} {
		latest, _ := Parse(tc.latest)
		running, _ := Parse(tc.running)
		if got := Newer(latest, running); got != tc.want {
			t.Errorf("Newer(%s, %s) = %v", tc.latest, tc.running, got)
		}
	}
}

// server stands in for GitHub: the API and the release's downloads.
func server(t *testing.T, status int, body string, files map[string]string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != userAgent {
			t.Errorf("user agent %q", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("credentials sent")
		}
		if r.URL.Path == "/Dardrai/airpods-gui/releases/latest" {
			w.Header().Set("Location", srv.URL+"/Dardrai/airpods-gui/releases/tag/v0.3.0-beta")
			w.WriteHeader(http.StatusFound)
			return
		}
		if name, ok := strings.CutPrefix(r.URL.Path, "/Dardrai/airpods-gui/releases/download/v0.3.0-beta/"); ok {
			r.URL.Path = "/download/" + name
		}
		if name, ok := strings.CutPrefix(r.URL.Path, "/download/"); ok {
			data, found := files[name]
			if !found {
				http.NotFound(w, r)
				return
			}
			fmt.Fprint(w, data)
			return
		}
		if r.URL.Path != "/repos/Dardrai/airpods-gui/releases/latest" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.WriteHeader(status)
		fmt.Fprint(w, strings.ReplaceAll(body, "SERVER", srv.URL))
	}))
	oldAPI, oldWeb := apiBase, webBase
	apiBase, webBase = srv.URL, srv.URL
	t.Cleanup(func() { apiBase, webBase = oldAPI, oldWeb; srv.Close() })
	return srv
}

const releaseJSON = `{"tag_name":"v0.3.0-beta","html_url":"https://github.com/Dardrai/airpods-gui/releases/tag/v0.3.0-beta",
 "draft":false,"assets":[
  {"name":"airpods-gui-0.3.0-beta-setup-x64.exe","browser_download_url":"SERVER/download/airpods-gui-0.3.0-beta-setup-x64.exe","size":5},
  {"name":"SHA256SUMS","browser_download_url":"SERVER/download/SHA256SUMS","size":100}]}`

func TestLatest(t *testing.T) {
	srv := server(t, 200, releaseJSON, nil)
	rel, err := Latest(context.Background(), srv.Client(), Repo)
	if err != nil {
		t.Fatal(err)
	}
	if rel.Tag != "v0.3.0-beta" || rel.Version.Minor != 3 || len(rel.Assets) != 2 {
		t.Errorf("%+v", rel)
	}
}

func TestLatestFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   error
	}{
		"none":     {404, `{"message":"Not Found"}`, ErrNoRelease},
		"garbage":  {200, `not json`, nil},
		"bad tag":  {200, `{"tag_name":"nightly","html_url":"https://github.com/x"}`, nil},
		"bad page": {200, `{"tag_name":"v1.0.0","html_url":"https://evil.example/x"}`, nil},
		"draft":    {200, `{"tag_name":"v1.0.0","html_url":"https://github.com/x","draft":true}`, nil},
	} {
		t.Run(name, func(t *testing.T) {
			srv := server(t, tc.status, tc.body, nil)
			_, err := Latest(context.Background(), srv.Client(), Repo)
			if err == nil || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Errorf("err %v", err)
			}
		})
	}
}

func sum(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

func TestDownloadChecksTheSum(t *testing.T) {
	name := InstallerName("v0.3.0-beta")
	for label, tc := range map[string]struct {
		sums string
		ok   bool
	}{
		"match":    {sum("setup") + "  " + name + "\n" + sum("x") + "  other\n", true},
		"binary":   {sum("setup") + " *" + name + "\n", true},
		"mismatch": {sum("other") + "  " + name + "\n", false},
		"missing":  {sum("setup") + "  other.exe\n", false},
	} {
		t.Run(label, func(t *testing.T) {
			srv := server(t, 200, releaseJSON, map[string]string{name: "setup", "SHA256SUMS": tc.sums})
			rel, err := Latest(context.Background(), srv.Client(), Repo)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			path, err := Download(context.Background(), srv.Client(), rel, dir, name)
			if tc.ok != (err == nil) {
				t.Fatalf("err %v", err)
			}
			if !tc.ok {
				if entries, _ := os.ReadDir(dir); len(entries) != 0 {
					t.Error("a file that does not match was left behind")
				}
				return
			}
			if data, _ := os.ReadFile(path); string(data) != "setup" || filepath.Base(path) != name {
				t.Errorf("%s: %q", path, data)
			}
		})
	}
}

func TestDownloadRefusesPlainHTTP(t *testing.T) {
	rel := Release{Tag: "v1.0.0", Assets: []Asset{
		{Name: InstallerName("v1.0.0"), URL: "http://example.com/a"}, {Name: sumsAsset, URL: "http://example.com/b"},
	}}
	if _, err := Download(context.Background(), http.DefaultClient, rel, t.TempDir(), InstallerName("v1.0.0")); err == nil {
		t.Error("downloaded over http")
	}
}

type memPrefs struct {
	mu sync.Mutex
	m  map[string]any
}

func (p *memPrefs) get(k string) (any, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	v, ok := p.m[k]
	return v, ok
}
func (p *memPrefs) set(k string, v any) { p.mu.Lock(); p.m[k] = v; p.mu.Unlock() }
func (p *memPrefs) BoolWithFallback(k string, f bool) bool {
	if v, ok := p.get(k); ok {
		return v.(bool)
	}
	return f
}
func (p *memPrefs) IntWithFallback(k string, f int) int {
	if v, ok := p.get(k); ok {
		return v.(int)
	}
	return f
}
func (p *memPrefs) StringWithFallback(k, f string) string {
	if v, ok := p.get(k); ok {
		return v.(string)
	}
	return f
}
func (p *memPrefs) SetBool(k string, v bool)     { p.set(k, v) }
func (p *memPrefs) SetInt(k string, v int)       { p.set(k, v) }
func (p *memPrefs) SetString(k string, v string) { p.set(k, v) }

func newChecker(prefs *memPrefs, running string, fetches *int, mu *sync.Mutex, news *[]string) *Checker {
	latest, _ := Parse("v0.3.0-beta")
	return &Checker{
		Prefs: prefs, Running: running,
		fetch: func(context.Context) (Release, error) {
			mu.Lock()
			*fetches++
			mu.Unlock()
			return Release{Tag: "v0.3.0-beta", Page: "https://github.com/p", Version: latest}, nil
		},
		OnNew: func(r Release) { mu.Lock(); *news = append(*news, r.Tag); mu.Unlock() },
	}
}

func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestCheckerStaysOfflineUntilTurnedOn(t *testing.T) {
	prefs := &memPrefs{m: map[string]any{}}
	var mu sync.Mutex
	fetches, news := 0, []string{}
	c := newChecker(prefs, "v0.2.1-beta", &fetches, &mu, &news)
	c.Start()
	defer c.Stop()
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if fetches != 0 {
		t.Fatalf("%d requests while off", fetches)
	}
	mu.Unlock()
	c.SetEnabled(true)
	eventually(t, func() bool { return !c.Result().Checking && c.Result().Known })
	r := c.Result()
	if !r.Available || r.Latest.Tag != "v0.3.0-beta" {
		t.Errorf("%+v", r)
	}
	mu.Lock()
	if fetches != 1 || len(news) != 1 {
		t.Errorf("fetches %d, news %v", fetches, news)
	}
	mu.Unlock()
}

func TestCheckerAnnouncesATagOnceAndRemembersIt(t *testing.T) {
	prefs := &memPrefs{m: map[string]any{Preference: true}}
	var mu sync.Mutex
	fetches, news := 0, []string{}
	c := newChecker(prefs, "v0.2.1-beta", &fetches, &mu, &news)
	c.Start()
	eventually(t, func() bool { return c.Result().Known && !c.Result().Checking })
	c.Stop()

	// A restart within the day does not ask again, and shows what was seen.
	again := newChecker(prefs, "v0.2.1-beta", &fetches, &mu, &news)
	again.Start()
	defer again.Stop()
	if r := again.Result(); !r.Known || !r.Available {
		t.Errorf("not remembered: %+v", r)
	}
	again.CheckNow()
	eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return fetches == 2
	})
	eventually(t, func() bool { return !again.Result().Checking })
	mu.Lock()
	defer mu.Unlock()
	if len(news) != 1 {
		t.Errorf("announced %v", news)
	}
}

func TestDevBuildIsNeverBehind(t *testing.T) {
	prefs := &memPrefs{m: map[string]any{Preference: true}}
	var mu sync.Mutex
	fetches, news := 0, []string{}
	c := newChecker(prefs, "", &fetches, &mu, &news)
	c.Start()
	defer c.Stop()
	eventually(t, func() bool { return c.Result().Known && !c.Result().Checking })
	if r := c.Result(); r.Available || !r.Dev {
		t.Errorf("%+v", r)
	}
	if len(news) != 0 {
		t.Errorf("announced %v", news)
	}
}

func TestInstallerKeysMatch(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "build", "windows", "installer.nsi"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`!define UNINSTALL_KEY "` + uninstallKey + `"`, `"/RELAUNCH"`} {
		if !strings.Contains(string(script), want) {
			t.Errorf("build/windows/installer.nsi has no %s", want)
		}
	}
}

// Past the API's hourly limit, the tag comes from the website's redirect, and the files from
// GitHub's download addresses, still checked against SHA256SUMS.
func TestLatestPastTheRateLimit(t *testing.T) {
	name := InstallerName("v0.3.0-beta")
	srv := server(t, 403, `{"message":"API rate limit exceeded"}`, map[string]string{name: "setup", "SHA256SUMS": sum("setup") + "  " + name + "\n"})
	rel, err := Latest(context.Background(), srv.Client(), Repo)
	if err != nil {
		t.Fatal(err)
	}
	if rel.Tag != "v0.3.0-beta" || rel.Page != "https://github.com/Dardrai/airpods-gui/releases/tag/v0.3.0-beta" {
		t.Errorf("%+v", rel)
	}
	path, err := Download(context.Background(), srv.Client(), rel, t.TempDir(), name)
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "setup" {
		t.Errorf("%q", data)
	}
}

// fakeSystem answers detection: tools on the PATH, and what the package managers say.
func fakeSystem(exe string, tools map[string]string) system {
	return system{
		exe: exe, arch: "amd64",
		lookPath: func(name string) (string, error) {
			if out, ok := tools[name]; ok && out == "missing" {
				return "", errors.New("not found")
			}
			if _, ok := tools[name]; ok || name == "pkexec" {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
		output: func(name string, args ...string) (string, error) {
			if out, ok := tools[name]; ok && out != "" {
				return out + "\n", nil
			}
			return "", errors.New("not owned")
		},
	}
}

func TestLinuxPlan(t *testing.T) {
	for label, tc := range map[string]struct {
		exe   string
		tools map[string]string
		kind  string
		asset string
		cmd   string
	}{
		"arch":         {linuxExe, map[string]string{"pacman": "airpods-gui"}, "pacman", "airpods-gui_0.3.1-beta-x86_64.pkg.tar.zst", "pkexec pacman -U --noconfirm"},
		"debian":       {linuxExe, map[string]string{"dpkg-query": "airpods-gui: /usr/bin/airpods-tray", "apt-get": ""}, "apt", "airpods-gui_0.3.1-beta_amd64.deb", "pkexec apt-get install -y --allow-downgrades"},
		"fedora":       {linuxExe, map[string]string{"rpm": "airpods-gui", "dnf": ""}, "dnf", "airpods-gui_0.3.1-beta.x86_64.rpm", "pkexec dnf install -y"},
		"opensuse":     {linuxExe, map[string]string{"rpm": "airpods-gui", "zypper": ""}, "zypper", "airpods-gui_0.3.1-beta.x86_64.rpm", "pkexec zypper --non-interactive install --allow-unsigned-rpm"},
		"other owner":  {linuxExe, map[string]string{"pacman": "something-else"}, "", "", ""},
		"plain binary": {"/home/u/bin/airpods-tray", map[string]string{"pacman": "airpods-gui"}, "", "", ""},
		"no pkexec":    {linuxExe, map[string]string{"pacman": "airpods-gui", "pkexec": "missing"}, "", "", ""},
	} {
		t.Run(label, func(t *testing.T) {
			kind, asset, cmd, ok := linuxPlan(fakeSystem(tc.exe, tc.tools))
			if ok != (tc.kind != "") || kind != tc.kind {
				t.Fatalf("kind %q ok %v", kind, ok)
			}
			if !ok {
				return
			}
			if got := asset("v0.3.1-beta"); got != tc.asset {
				t.Errorf("asset %q", got)
			}
			if got := strings.Join(cmd, " "); got != tc.cmd {
				t.Errorf("command %q", got)
			}
		})
	}
}

// The package names match what build/fyne-cross.sh and the release name.
func TestPackageNamesMatchTheBuild(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "build", "fyne-cross.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"${base}_amd64.deb"`, `"$base.x86_64.rpm"`, `"$base-x86_64.pkg.tar.zst"`, `local base="dist/airpods-gui_${version#v}"`} {
		if !strings.Contains(string(script), want) {
			t.Errorf("build/fyne-cross.sh has no %s", want)
		}
	}
}
