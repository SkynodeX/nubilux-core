package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

var (
	isServerRunning bool
	serverMutex     sync.Mutex
	lastActive      time.Time
	javaCmd         *exec.Cmd
	javaStdin       io.WriteCloser
	internalPort    string
)

func startProxyEngine() {
	serverPort := os.Getenv("SERVER_PORT")
	if serverPort == "" {
		serverPort = "25565"
	}
	
	portNum, _ := strconv.Atoi(serverPort)
	internalPort = strconv.Itoa(portNum + 10000)

	listener, err := net.Listen("tcp", "0.0.0.0:"+serverPort)
	if err != nil {
		log.Fatalf("[\033[31mError\033[0m] Failed to bind proxy to port %s: %v", serverPort, err)
	}
	log.Printf("[\033[36mNubilux Hibernation Proxy\033[0m] Listening on :%s", serverPort)
	log.Printf("[\033[32mNubilux\033[0m] Server is hibernating to save resources! (0MB RAM)")
	log.Printf("[\033[32mNubilux\033[0m] To wake up the server, simply connect to it in Minecraft.")
	fmt.Println("Done (hibernating)") // Tells Pterodactyl the server has finished starting

	// Background idle checker
	go func() {
		for {
			time.Sleep(1 * time.Minute)
			serverMutex.Lock()
			// If running and no connections for 15 minutes, shut down
			if isServerRunning && time.Since(lastActive) > 15*time.Minute {
				log.Println("[\033[36mNubilux Hibernation Proxy\033[0m] Server idle for 15 minutes. Hibernating (0 CPU/RAM)...")
				if javaCmd != nil && javaCmd.Process != nil {
					io.WriteString(javaStdin, "stop\n")
					
					go func() {
						javaCmd.Wait()
						serverMutex.Lock()
						isServerRunning = false
						serverMutex.Unlock()
						log.Println("[\033[36mNubilux Hibernation Proxy\033[0m] Server successfully hibernated.")
					}()
				}
			}
			serverMutex.Unlock()
		}
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go handleConnection(conn)
	}
}

func runJavaServer() {
	memStr := os.Getenv("SERVER_MEMORY")
	memSize := 1024
	if memStr != "" {
		parsed, err := strconv.Atoi(memStr)
		if err == nil {
			memSize = parsed
		}
	}

	args := []string{
		fmt.Sprintf("-Xms%dM", memSize),
		fmt.Sprintf("-Xmx%dM", memSize),
		"-XX:+UseG1GC",
		"-XX:+ParallelRefProcEnabled",
		"-XX:MaxGCPauseMillis=200",
		"-XX:+UnlockExperimentalVMOptions",
		"-XX:+DisableExplicitGC",
		"-XX:+AlwaysPreTouch",
		"-XX:G1NewSizePercent=30",
		"-XX:G1MaxNewSizePercent=40",
		"-XX:G1HeapRegionSize=8M",
		"-XX:G1ReservePercent=20",
		"-XX:G1HeapWastePercent=5",
		"-XX:G1MixedGCCountTarget=4",
		"-XX:InitiatingHeapOccupancyPercent=15",
		"-XX:G1MixedGCLiveThresholdPercent=90",
		"-XX:G1RSetUpdatingPauseTimePercent=5",
		"-XX:SurvivorRatio=32",
		"-XX:+PerfDisableSharedMem",
		"-XX:MaxTenuringThreshold=1",
		"-Dusing.aikars.flags=https://mcflags.emc.gs",
		"-Daikars.new.flags=true",
		"-Dserver.port=" + internalPort, // Bind internally
		"-Dserver.ip=127.0.0.1",
		"-jar", "core.jar", "nogui",
	}

	javaCmd = exec.Command("java", args...)
	javaCmd.Stdout = os.Stdout
	javaCmd.Stderr = os.Stderr
	
	stdin, err := javaCmd.StdinPipe()
	if err == nil {
		javaStdin = stdin
	}

	log.Println("[\033[36mNubilux\033[0m] Booting Java Engine...")
	err = javaCmd.Start()
	if err != nil {
		log.Printf("[\033[31mError\033[0m] Failed to start engine: %v", err)
		return
	}
	
	isServerRunning = true
	lastActive = time.Now()
}

func handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	serverMutex.Lock()
	lastActive = time.Now()
	
	if !isServerRunning {
		log.Println("[\033[36mNubilux Hibernation Proxy\033[0m] Incoming connection detected. Waking up engine...")
		runJavaServer()
		// Wait for internal port to open (simple poll)
		for i := 0; i < 20; i++ {
			time.Sleep(1 * time.Second)
			testConn, err := net.Dial("tcp", "127.0.0.1:"+internalPort)
			if err == nil {
				testConn.Close()
				break
			}
		}
	}
	serverMutex.Unlock()

	serverConn, err := net.Dial("tcp", "127.0.0.1:"+internalPort)
	if err != nil {
		log.Println("[\033[31mError\033[0m] Failed to proxy connection to engine.")
		return
	}
	defer serverConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		io.Copy(serverConn, clientConn)
		wg.Done()
	}()
	go func() {
		io.Copy(clientConn, serverConn)
		wg.Done()
	}()

	wg.Wait()
}
