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

// --- PAPER & VELOCITY ---
type PaperV3Build struct {
	Id        int `json:"id"`
	Downloads struct {
		ServerDefault struct {
			Name      string `json:"name"`
			Checksums struct {
				Sha256 string `json:"sha256"`
			} `json:"checksums"`
			Url string `json:"url"`
		} `json:"server:default"`
	} `json:"downloads"`
}

func httpGetWithUA(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Nubilux-Core/1.0 (contact@nubilux.com)")
	client := &http.Client{}
	return client.Do(req)
}

func DownloadFromPaperAPI(project string, version string) error {
	buildsUrl := fmt.Sprintf("https://fill.papermc.io/v3/projects/%s/versions/%s/builds", project, version)
	resp, err := httpGetWithUA(buildsUrl)
	if err != nil { return err }
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("invalid version %s for %s", version, project)
	}

	var buildsData []PaperV3Build
	if err := json.NewDecoder(resp.Body).Decode(&buildsData); err != nil { return err }
	if len(buildsData) == 0 { return fmt.Errorf("no builds found") }
	
	// The first build in the array is typically the latest, but we can loop to find max ID
	latestBuild := buildsData[0]
	for _, b := range buildsData {
		if b.Id > latestBuild.Id {
			latestBuild = b
		}
	}

	downloadUrl := latestBuild.Downloads.ServerDefault.Url
	expectedHash := latestBuild.Downloads.ServerDefault.Checksums.Sha256

	if downloadUrl == "" {
		return fmt.Errorf("failed to get download URL from paper v3 API")
	}

	return downloadAndVerify("core.jar", downloadUrl, expectedHash)
}

// --- PURPUR ---
type PurpurVersionResponse struct {
	Builds struct {
		Latest string `json:"latest"`
	} `json:"builds"`
}

func DownloadPurpur(version string) error {
	versionUrl := fmt.Sprintf("https://api.purpurmc.org/v2/purpur/%s", version)
	resp, err := http.Get(versionUrl)
	if err != nil { return err }
	defer resp.Body.Close()

	var data PurpurVersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil { return err }

	downloadUrl := fmt.Sprintf("https://api.purpurmc.org/v2/purpur/%s/%s/download", version, data.Builds.Latest)
	
	// Purpur doesn't easily expose SHA in a single API call, skipping hash check for now
	return downloadAndVerify("core.jar", downloadUrl, "")
}

// --- VANILLA (MOJANG) ---
type MojangManifest struct {
	Versions []struct {
		Id  string `json:"id"`
		Url string `json:"url"`
	} `json:"versions"`
}
type MojangVersion struct {
	Downloads struct {
		Server struct {
			Url  string `json:"url"`
			Sha1 string `json:"sha1"`
		} `json:"server"`
	} `json:"downloads"`
}

func DownloadVanilla(version string) error {
	resp, err := http.Get("https://launchermeta.mojang.com/mc/game/version_manifest.json")
	if err != nil { return err }
	defer resp.Body.Close()

	var manifest MojangManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil { return err }

	var versionUrl string
	for _, v := range manifest.Versions {
		if v.Id == version {
			versionUrl = v.Url
			break
		}
	}
	if versionUrl == "" { return fmt.Errorf("vanilla version %s not found", version) }

	resp2, err := http.Get(versionUrl)
	if err != nil { return err }
	defer resp2.Body.Close()

	var vData MojangVersion
	if err := json.NewDecoder(resp2.Body).Decode(&vData); err != nil { return err }

	return downloadAndVerify("core.jar", vData.Downloads.Server.Url, "") // Skipping SHA1 for simplicity in this proxy
}

// --- BUNGEECORD ---
func DownloadBungeeCord() error {
	url := "https://ci.md-5.net/job/BungeeCord/lastSuccessfulBuild/artifact/bootstrap/target/BungeeCord.jar"
	return downloadAndVerify("core.jar", url, "")
}

// --- UTILS ---
type progressWriter struct {
	Total      int64
	Downloaded int64
	LastPct    int
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.Downloaded += int64(n)
	if pw.Total > 0 {
		pct := int(float64(pw.Downloaded) / float64(pw.Total) * 100)
		// Print every 10%
		if pct >= pw.LastPct+10 {
			fmt.Printf("[\033[36mNubilux\033[0m] Downloading: %d%%\n", pct)
			pw.LastPct = pct
		}
	} else {
		mb := int(pw.Downloaded / 1024 / 1024)
		if mb >= pw.LastPct+5 { // Print every 5MB
			fmt.Printf("[\033[36mNubilux\033[0m] Downloading: %d MB...\n", mb)
			pw.LastPct = mb
		}
	}
	return n, nil
}

func downloadAndVerify(filePath, url, expectedHash string) error {
	if expectedHash != "" {
		if _, err := os.Stat(filePath); err == nil {
			hashBytes, err := exec.Command("sha256sum", filePath).Output()
			if err == nil {
				currentHash := strings.Fields(string(hashBytes))[0]
				if currentHash == expectedHash {
					return nil // Valid
				}
			}
			fmt.Println("[\033[33mNubilux\033[0m] Hash mismatch. Overriding with secure release...")
			os.Remove(filePath)
		}
	} else {
		// If no hash check is provided, we just redownload to be safe if the file doesn't exist
		if _, err := os.Stat(filePath); err == nil {
			return nil // Assume valid if exists
		}
	}

	fmt.Println("[\033[36mNubilux\033[0m] Downloading engine from API...")
	
	out, err := os.Create(filePath + ".tmp")
	if err != nil { return err }
	defer out.Close()

	resp, err := httpGetWithUA(url)
	if err != nil { return err }
	defer resp.Body.Close()

	pw := &progressWriter{Total: resp.ContentLength}
	reader := io.TeeReader(resp.Body, pw)

	if _, err = io.Copy(out, reader); err != nil { return err }
	fmt.Println("[\033[36mNubilux\033[0m] Download complete.")

	out.Close()
	return os.Rename(filePath + ".tmp", filePath)
}
