//go:build pi4 || rock5a

package upgrade

import (
	"log"
	"os"
	"runtime/debug"

	"github.com/blang/semver"
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

func ConfirmAndSelfUpdate() {
	currentVersion := getVersion()
	filters := assetFilters()
	log.Printf("Self-update target: %s (asset filter: %v)", boardName(), filters)

	if currentVersion == "(devel)" || currentVersion == "unknown" {
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

	currentSemVer := semver.MustParse(currentVersion)
	if latest.Version.Equals(currentSemVer) || !latest.Version.GT(currentSemVer) {
		log.Println("Current version is the latest")
		return
	}

	log.Println("New version available:", latest.Version)

	if _, err = updater.UpdateSelf(currentSemVer, githubRepo); err != nil {
		log.Println("Error occurred while updating binary:", err)
		return
	}

	log.Println("Successfully updated to version", latest.Version)
	os.Exit(1)
}
