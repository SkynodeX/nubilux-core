package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type PaperVersionResponse struct {
	Builds []int `json:"builds"`
}

type PaperBuildResponse struct {
	Downloads struct {
		Application struct {
			Name   string `json:"name"`
			Sha256 string `json:"sha256"`
		} `json:"application"`
	} `json:"downloads"`
}

// DownloadPaper fetches the latest build of the specified version from PaperMC API
func DownloadPaper(version string) error {
	versionUrl := fmt.Sprintf("https://api.papermc.io/v2/projects/paper/versions/%s", version)
	resp, err := http.Get(versionUrl)
	if err != nil {
		return fmt.Errorf("failed to fetch paper version: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("invalid version %s (status %d)", version, resp.StatusCode)
	}

	var versionData PaperVersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&versionData); err != nil {
		return fmt.Errorf("failed to decode version data: %v", err)
	}

	if len(versionData.Builds) == 0 {
		return fmt.Errorf("no builds found for version %s", version)
	}
	
	latestBuild := versionData.Builds[len(versionData.Builds)-1]

	buildUrl := fmt.Sprintf("https://api.papermc.io/v2/projects/paper/versions/%s/builds/%d", version, latestBuild)
	resp2, err := http.Get(buildUrl)
	if err != nil {
		return fmt.Errorf("failed to fetch build details: %v", err)
	}
	defer resp2.Body.Close()

	var buildData PaperBuildResponse
	if err := json.NewDecoder(resp2.Body).Decode(&buildData); err != nil {
		return fmt.Errorf("failed to decode build data: %v", err)
	}

	fileName := buildData.Downloads.Application.Name
	expectedHash := buildData.Downloads.Application.Sha256
	downloadUrl := fmt.Sprintf("https://api.papermc.io/v2/projects/paper/versions/%s/builds/%d/downloads/%s", version, latestBuild, fileName)

	// Check if core.jar already exists and matches the expected hash
	if _, err := os.Stat("core.jar"); err == nil {
		hashBytes, err := exec.Command("sha256sum", "core.jar").Output()
		if err == nil {
			currentHash := strings.Fields(string(hashBytes))[0]
			if currentHash == expectedHash {
				return nil
			}
		}
		fmt.Println("[\033[33mNubilux\033[0m] Custom or outdated JAR detected. Overriding with official secure release...")
		os.Remove("core.jar")
	}

	fmt.Printf("[\033[36mNubilux\033[0m] Downloading Paper %s build %d...\n", version, latestBuild)
	
	out, err := os.Create("core.jar.tmp")
	if err != nil {
		return err
	}
	defer out.Close()

	resp3, err := http.Get(downloadUrl)
	if err != nil {
		return err
	}
	defer resp3.Body.Close()

	_, err = io.Copy(out, resp3.Body)
	if err != nil {
		return err
	}

	out.Close()
	return os.Rename("core.jar.tmp", "core.jar")
}
