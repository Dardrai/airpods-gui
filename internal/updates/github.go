package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Repo is where the app's releases are published.
const Repo = "Dardrai/airpods-gui"

// apiBase is GitHub's REST API, and webBase its website; tests point them elsewhere.
var (
	apiBase = "https://api.github.com"
	webBase = "https://github.com"
)

const (
	// requestTimeout bounds one call to GitHub.
	requestTimeout = 10 * time.Second
	// maxReleaseJSON bounds the release description the app reads.
	maxReleaseJSON = 1 << 20
	userAgent      = "airpods-gui"
)

// Asset is one file attached to a release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// Release is the newest published release.
type Release struct {
	Tag     string  `json:"tag_name"`
	Page    string  `json:"html_url"`
	Draft   bool    `json:"draft"`
	Assets  []Asset `json:"assets"`
	Version Version `json:"-"`
}

// Asset finds an attached file by name.
func (r Release) Asset(match func(name string) bool) (Asset, bool) {
	for _, a := range r.Assets {
		if match(a.Name) {
			return a, true
		}
	}
	return Asset{}, false
}

// ErrNoRelease means the repository has no published release.
var ErrNoRelease = errors.New("updates: no published release")

// Latest asks GitHub for the repository's newest release that is not a draft or a
// pre-release. It sends no credentials and nothing about the user.
func Latest(ctx context.Context, client *http.Client, repo string) (Release, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return Release{}, ErrNoRelease
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		// The API allows 60 requests an hour per address without an account, shared by
		// everyone behind the same router. The website's redirect has no such limit.
		return latestFromWeb(ctx, client, repo)
	case resp.StatusCode != http.StatusOK:
		return Release{}, fmt.Errorf("updates: GitHub answered %s", resp.Status)
	}
	var rel Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseJSON)).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("updates: unreadable release: %w", err)
	}
	v, ok := Parse(rel.Tag)
	if !ok || rel.Draft {
		return Release{}, fmt.Errorf("updates: unexpected release tag %q", rel.Tag)
	}
	if !strings.HasPrefix(rel.Page, "https://github.com/") {
		return Release{}, fmt.Errorf("updates: unexpected release page %q", rel.Page)
	}
	rel.Version = v
	return rel, nil
}

// latestFromWeb reads the newest release's tag from where github.com redirects
// /releases/latest, and names its files by GitHub's download address for that tag.
func latestFromWeb(ctx context.Context, client *http.Client, repo string) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, webBase+"/"+repo+"/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	noFollow := *client
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := noFollow.Do(req)
	if err != nil {
		return Release{}, err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusMovedPermanently {
		return Release{}, fmt.Errorf("updates: GitHub answered %s", resp.Status)
	}
	page := resp.Header.Get("Location")
	prefix := webBase + "/" + repo + "/releases/tag/"
	tag, ok := strings.CutPrefix(page, prefix)
	if !ok {
		// No release: GitHub sends /releases instead.
		return Release{}, ErrNoRelease
	}
	v, ok := Parse(tag)
	if !ok {
		return Release{}, fmt.Errorf("updates: unexpected release tag %q", tag)
	}
	download := webBase + "/" + repo + "/releases/download/" + tag + "/"
	rel := Release{Tag: tag, Page: "https://github.com/" + repo + "/releases/tag/" + tag, Version: v}
	for _, name := range []string{InstallerName(tag), sumsAsset} {
		rel.Assets = append(rel.Assets, Asset{Name: name, URL: download + name})
	}
	return rel, nil
}
