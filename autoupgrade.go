package main

import (
	"log"
	"os"
	"runtime/debug"

	"github.com/blang/semver"
	"github.com/joho/godotenv"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
)

// appVersion はビルド時に埋め込まれるバージョン情報である
var appVersion string

const (
	githubRepo = "Rione/ssl-RACOON-Pi2"
)

// getVersion は現在のアプリケーションバージョンを取得する
func getVersion() string {
	if appVersion != "" {
		return appVersion
	}

	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	log.Println("Build version:", buildInfo.Main.Version)
	return buildInfo.Main.Version
}

// confirmAndSelfUpdate はGitHubから最新版を確認し、必要に応じて自動更新を行う
func confirmAndSelfUpdate() {
	currentVersion := getVersion()
	log.Println("Current version:", currentVersion)

	if currentVersion == "(devel)" || currentVersion == "unknown" {
		log.Println("NO VERSION INFO (DEV VERSION)")
		return
	}

	// .envファイルからGitHubトークンを読み込む
	if err := godotenv.Load(); err != nil {
		log.Println("Error loading .env file:", err)
		return
	}

	updater, err := selfupdate.NewUpdater(selfupdate.Config{
		APIToken: os.Getenv("GITHUB_TOKEN"),
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
