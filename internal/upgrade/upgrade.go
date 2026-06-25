//go:build pi4 || rock5a

package upgrade

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/blang/semver"
	"github.com/inconshreveable/go-update"
	"github.com/joho/godotenv"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
)

var Version string

const githubRepo = "Rione/ssl-RACOON-Pi2"

func getVersion() string {
	if Version != "" {
		return Version
	}

	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	log.Println("Build version:", buildInfo.Main.Version)
	return buildInfo.Main.Version
}

func normalizeVersion(v string) string {
	return strings.TrimPrefix(v, "v")
}

func isDevVersion(v string) bool {
	switch v {
	case "", "(devel)", "unknown":
		return true
	}
	// go build 由来の pseudo-version (例: v1.0.1-0.20260623143125-eab18fdc67ec)
	return strings.Contains(normalizeVersion(v), "-0.")
}

func parseCurrentVersion(v string) (semver.Version, bool) {
	parsed, err := semver.Parse(normalizeVersion(v))
	if err != nil {
		log.Printf("Skipping self-update: cannot parse version %q: %v", v, err)
		return semver.Version{}, false
	}
	return parsed, true
}

func ConfirmAndSelfUpdate() {
	currentVersion := getVersion()
	filters := assetFilters()
	log.Printf("Self-update target: %s (asset filter: %v)", boardName(), filters)

	if isDevVersion(currentVersion) {
		log.Println("NO VERSION INFO (DEV VERSION)")
		return
	}

	_ = godotenv.Load()

	updater, err := selfupdate.NewUpdater(selfupdate.Config{
		APIToken: os.Getenv("GITHUB_TOKEN"),
		Filters:  filters,
	})
	if err != nil {
		log.Println("Error occurred while creating the updater:", err)
		return
	}

	latest, found, err := updater.DetectLatest(githubRepo)
	if err != nil {
		log.Println("Error occurred while detecting version:", err)
		return
	}

	if !found {
		log.Println("No releases found")
		return
	}

	currentSemVer, ok := parseCurrentVersion(currentVersion)
	if !ok {
		return
	}
	if latest.Version.Equals(currentSemVer) || !latest.Version.GT(currentSemVer) {
		log.Println("Current version is the latest")
		return
	}

	log.Println("New version available:", latest.Version)

	cmdPath, err := os.Executable()
	if err != nil {
		log.Println("Error occurred while resolving executable path:", err)
		return
	}

	if err = applyRelease(latest, cmdPath); err != nil {
		log.Println("Error occurred while updating binary:", err)
		return
	}

	log.Println("Successfully updated to version", latest.Version)
	os.Exit(1)
}

func archiveBinaryNames(cmdPath string) []string {
	seen := make(map[string]bool)
	var names []string
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
	}

	add(archiveBinaryName())
	add(filepath.Base(cmdPath))
	add("ssl-RACOON-Pi2")
	add("racoon-pi2")
	return names
}

func fetchReleaseAsset(rel *selfupdate.Release) ([]byte, error) {
	resp, err := http.Get(rel.AssetURL)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", rel.AssetURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: status %d", rel.AssetURL, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func applyRelease(rel *selfupdate.Release, targetPath string) error {
	data, err := fetchReleaseAsset(rel)
	if err != nil {
		return err
	}

	var lastErr error
	for _, name := range archiveBinaryNames(targetPath) {
		asset, err := selfupdate.UncompressCommand(bytes.NewReader(data), rel.AssetURL, name)
		if err != nil {
			lastErr = err
			continue
		}
		log.Printf("Updating %s using archive binary %q", targetPath, name)
		if err := update.Apply(asset, update.Options{TargetPath: targetPath}); err != nil {
			return err
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("no archive binary name candidates for %s", targetPath)
}
