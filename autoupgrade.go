package main

import (
	"log"
	"os"
	"runtime/debug"

	"github.com/joho/godotenv"

	"github.com/blang/semver"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
)

var version string

func getVersion() string {
	if version != "" {
		// バージョン情報が埋め込まれている時
		return version
	}
	i, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	return i.Main.Version
}

func confirmAndSelfUpdate() {
	var version = getVersion()
	log.Println("Current version:", version)

	if version == "(devel)" || version == "unknown" {
		log.Println("NO VERSION INFO(DEV VERSION)")
		return
	}

	// selfupdate.EnableLog()
	// .envファイルを読み込む
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file")
		return
	}
	up, err := selfupdate.NewUpdater(selfupdate.Config{
		APIToken: os.Getenv("GITHUB_TOKEN"),
	})
	if err != nil {
		log.Println("Error occurred while creating the updater:", err)
		return
	}

	latest, found, err := up.DetectLatest("Rione/ssl-RACOON-Pi2")
	if err != nil {
		log.Println("Error occurred while detecting version:", err)
		return
	}
	if !found {
		log.Println("No releases found")
		return
	}

	v := semver.MustParse(version)
	if !found || latest.Version.Equals(v) || !latest.Version.GT(v) {
		log.Println("Current version is the latest")
		return
	}
	log.Println("New version available:", latest.Version)

	latest, err = up.UpdateSelf(v, "Rione/ssl-RACOON-Pi2")
	if err != nil {
		log.Println("Error occurred while updating binary:", err)
		return
	}
	log.Println("Successfully updated to version", latest.Version)

	os.Exit(1)
}
