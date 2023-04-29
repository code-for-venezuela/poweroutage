package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	configFile = "/etc/wpa_supplicant/wpa_supplicant.conf"
)

func main() {
	// Check if wifi is already configured and connected
	if wifiConfigured() {
		log.Println("Wifi is already configured and connected, exiting.")
		return
	}

	// Start Access Point
	log.Println("Starting Access Point...")
	if err := startAP(); err != nil {
		log.Fatalf("Failed to start Access Point: %v", err)
	}
	defer stopAP()

	// Serve Configuration Page
	http.HandleFunc("/", handleConfiguration)
	log.Println("Serving Configuration Page on http://0.0.0.0:8080")
	server := &http.Server{Addr: "0.0.0.0:8080"}
	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("Failed to serve Configuration Page: %v", err)
		}
	}()

	// Check if wifi is now configured and connected every 10 seconds
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if wifiConfigured() {
				log.Println("Wifi is now configured and connected, exiting.")
				server.Close()
				return
			}
			log.Println("Wifi is still not configured or connected, restarting.")
		}
	}
}

// wifiConfigured checks if wifi is already configured and connected by reading the wpa_supplicant.conf file
func wifiConfigured() bool {
	file, err := os.Open(configFile)
	if err != nil {
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ssid=") {
			ssid := strings.Trim(line[5:], "\"")
			cmd := exec.Command("iwgetid", "-r")
			output, err := cmd.Output()
			if err == nil {
				if strings.TrimSpace(string(output)) == ssid {
					return true
				}
			}
			break
		}
	}

	return false
}

// startAP starts an Access Point with SSID "RaspberryAP" and password "raspberry"
func startAP() error {
	cmd := exec.Command("sudo", "systemctl", "start", "hostapd.service")
	return cmd.Run()
}

// stopAP stops the Access Point
func stopAP() {
	cmd := exec.Command("sudo", "systemctl", "stop", "hostapd.service")
	cmd.Run()
}

// handleConfiguration serves the Configuration Page and tries to configure wifi based on user input
func handleConfiguration(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		fmt.Fprintf(w, "<html><body><h2>Wifi Configuration</h2><form method='POST'><label>SSID: <input type='text' name='ssid'></label><br><label>Password: <input type='text' name='password'></label><br><input type='submit' value='Submit'></form></body></html>")
	} else if r.Method == "POST" {
		ssid := r.FormValue("ssid")
		password := r.FormValue("password")
		if len(ssid) == 0 || len(password) == 0 {
			fmt.Fprintln(w, "SSID or Password cannot be empty.")
			return
		}

		// Update wpa_supplicant.conf file
		file, err := os.OpenFile(configFile, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			fmt.Fprintln(w, "Failed to update wpa_supplicant.conf file.")
			return
		}
		defer file.Close()

		if _, err := file.WriteString(fmt.Sprintf("\n\nnetwork={\n    ssid=\"%s\"\n    psk=\"%s\"\n}\n", ssid, password)); err != nil {
			fmt.Fprintln(w, "Failed to update wpa_supplicant.conf file.")
			return
		}

		// Restart networking service
		cmd := exec.Command("sudo", "systemctl", "restart", "dhcpcd.service")
		if err := cmd.Run(); err != nil {
			fmt.Fprintln(w, "Failed to restart networking service.")
			return
		}

		fmt.Fprintln(w, "Wifi configuration successful. Please reconnect to the Wifi network.")
	}
}
