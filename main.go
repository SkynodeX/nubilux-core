package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	wrapperVersion = "1.0.0"
	updateUrl      = "https://raw.githubusercontent.com/Nubilux/nubilux-core/main/versions.txt"
	binaryUrl      = "https://raw.githubusercontent.com/Nubilux/nubilux-core/main/nubilux-core"
)

func main() {
	log.Printf("[\033[36mNubilux\033[0m] Starting Nubilux Core Wrapper v%s...", wrapperVersion)

	// 1. Auto-Updater Check
	checkForUpdates()

	// 2. Accept EULA
	err := os.WriteFile("eula.txt", []byte("eula=true\n"), 0644)
	if err != nil {
		log.Fatalf("[\033[31mError\033[0m] Failed to write eula.txt: %v", err)
	}

	// 3. Download Core Engine
	serverSoftware := os.Getenv("SERVER_SOFTWARE")
	if serverSoftware == "" {
		serverSoftware = "paper"
	}
	minecraftVersion := os.Getenv("MINECRAFT_VERSION")
	if minecraftVersion == "" || minecraftVersion == "latest" {
		minecraftVersion = "1.20.4"
	}

	log.Printf("[\033[36mNubilux\033[0m] Verifying engine: %s %s", serverSoftware, minecraftVersion)
	if strings.ToLower(serverSoftware) == "paper" {
		err = DownloadPaper(minecraftVersion)
		if err != nil {
			log.Printf("[\033[33mWarning\033[0m] Engine download issue: %v", err)
		}
	} else {
		log.Printf("[\033[33mWarning\033[0m] %s is not fully supported for auto-download yet. Assuming core.jar exists.", serverSoftware)
	}

	// 4. Start Proxy / Hibernation Engine
	startProxyEngine()
}

func checkForUpdates() {
	if runtime.GOOS == "windows" {
		return // Skip auto-update testing on windows dev machine
	}
	
	resp, err := http.Get(updateUrl)
	if err != nil || resp.StatusCode != 200 {
		return // Ignore update if github is unreachable
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	
	latestVersion := strings.TrimSpace(string(bodyBytes))
	if latestVersion != "" && latestVersion != wrapperVersion {
		log.Printf("[\033[36mNubilux Updater\033[0m] New version v%s found! Updating...", latestVersion)
		
		// Download new binary
		out, err := os.Create("nubilux-core-new")
		if err == nil {
			dlResp, dlErr := http.Get(binaryUrl)
			if dlErr == nil {
				io.Copy(out, dlResp.Body)
				dlResp.Body.Close()
				out.Close()
				
				os.Chmod("nubilux-core-new", 0755)
				os.Rename("nubilux-core-new", os.Args[0])
				
				log.Println("[\033[36mNubilux Updater\033[0m] Update successful! Restarting wrapper...")
				
				// Restart
				cmd := exec.Command(os.Args[0], os.Args[1:]...)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				cmd.Stdin = os.Stdin
				cmd.Start()
				os.Exit(0)
			}
		}
	}
}
