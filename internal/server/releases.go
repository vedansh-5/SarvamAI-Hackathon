package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	UpdatePath      = "/v1/update/"
	UpdateFilePath  = "/v1/update-file/"
	DownloadPath    = "/v1/download"
	EnvDownloadLink = "VANS_DOWNLOAD_LINK"
	downloadLink    = "https://summonvans.in/download"
	EnvReleases     = "VANS_RELEASES_BUCKET"
	LatestKey       = "latest.json"
	DownloadLasts   = time.Hour
	mostLatestBytes = 64 << 10
)

type Release struct {
	Version   string           `json:"version"`
	Notes     string           `json:"notes"`
	PubDate   string           `json:"pub_date"`
	Platforms map[string]Build `json:"platforms"`
}

type Build struct {
	Key       string `json:"key"`
	Signature string `json:"signature"`
}

type Releases interface {
	Latest(ctx context.Context) (Release, error)
	Link(ctx context.Context, key string) (string, error)
}

type offered struct {
	Version   string `json:"version"`
	Notes     string `json:"notes,omitempty"`
	PubDate   string `json:"pub_date,omitempty"`
	URL       string `json:"url"`
	Signature string `json:"signature"`
}

func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	who, ok := s.tester(w, r)
	if !ok {
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, UpdatePath), "/")
	if len(parts) != 3 {
		refuse(w, http.StatusNotFound, "not found")
		return
	}
	platform, current := parts[0]+"-"+parts[1], parts[2]
	if s.Releases == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	latest, err := s.Releases.Latest(r.Context())
	if err != nil {
		s.logf("could not read the latest release: %v", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	build, has := latest.Platforms[platform]
	if !has || !newer(latest.Version, current) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.logf("%s is offered %s on %s, from %s", who.Name, latest.Version, platform, current)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(offered{
		Version: latest.Version, Notes: latest.Notes, PubDate: latest.PubDate,
		URL: selfURL(r) + UpdateFilePath + build.Key, Signature: build.Signature,
	})
}

func selfURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil && (strings.HasPrefix(r.Host, "127.0.0.1") || strings.HasPrefix(r.Host, "localhost")) {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

func (s *Server) updateFile(w http.ResponseWriter, r *http.Request) {
	who, ok := s.tester(w, r)
	if !ok {
		return
	}
	key := strings.TrimPrefix(r.URL.Path, UpdateFilePath)
	if s.Releases == nil || !releaseFile.MatchString(key) {
		refuse(w, http.StatusNotFound, "not found")
		return
	}
	link, err := s.Releases.Link(r.Context(), key)
	if err != nil {
		s.logf("could not make a download link for %s: %v", key, err)
		refuse(w, http.StatusServiceUnavailable, "the update could not be fetched; it is tried again later")
		return
	}
	s.logf("%s is downloading %s", who.Name, key)
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, link, http.StatusFound)
}

var releaseFile = regexp.MustCompile(`^\d+\.\d+\.\d+/VANS\.app\.tar\.gz$`)

func DmgKey(version string) string { return fmt.Sprintf("%s/VANS_%s.dmg", version, version) }

func DownloadLink() string {
	if link := strings.TrimSpace(os.Getenv(EnvDownloadLink)); link != "" {
		return link
	}
	return downloadLink
}

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	if s.Releases == nil {
		http.Error(w, "There is no build to download yet.", http.StatusServiceUnavailable)
		return
	}
	latest, err := s.Releases.Latest(r.Context())
	if err != nil {
		s.logf("could not read the latest release for a download: %v", err)
		http.Error(w, "Vspk could not be reached. Try again in a minute.", http.StatusServiceUnavailable)
		return
	}
	link, err := s.Releases.Link(r.Context(), DmgKey(latest.Version))
	if err != nil {
		s.logf("could not make a download link for %s: %v", latest.Version, err)
		http.Error(w, "Vspk could not be reached. Try again in a minute.", http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodGet {
		s.logf("someone downloaded %s", latest.Version)
	}
	http.Redirect(w, r, link, http.StatusFound)
}

func newer(latest, current string) bool {
	a, b := versionParts(latest), versionParts(current)
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func versionParts(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	for i, p := range strings.SplitN(v, ".", 3) {
		out[i], _ = strconv.Atoi(p)
	}
	return out
}

type S3API interface {
	GetObject(ctx context.Context, in *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

type S3Releases struct {
	Client  S3API
	Presign *s3.PresignClient
	Bucket  string
}

func (b *S3Releases) Latest(ctx context.Context) (Release, error) {
	out, err := b.Client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(b.Bucket), Key: aws.String(LatestKey)})
	if err != nil {
		return Release{}, err
	}
	defer out.Body.Close()
	data, err := io.ReadAll(io.LimitReader(out.Body, mostLatestBytes))
	if err != nil {
		return Release{}, err
	}
	var r Release
	if err := json.Unmarshal(data, &r); err != nil {
		return Release{}, fmt.Errorf("server: %s is not readable: %w", LatestKey, err)
	}
	return r, nil
}

func (b *S3Releases) Link(ctx context.Context, key string) (string, error) {
	req, err := b.Presign.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(b.Bucket), Key: aws.String(key)}, s3.WithPresignExpires(DownloadLasts))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func ReleasesFromEnv(cfg aws.Config) Releases {
	bucket := strings.TrimSpace(os.Getenv(EnvReleases))
	if bucket == "" {
		return nil
	}
	client := s3.NewFromConfig(cfg)
	return &S3Releases{Client: client, Presign: s3.NewPresignClient(client), Bucket: bucket}
}
