package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"encoding/json"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)
// setVSCodeProxy sets or clears the proxy in VS Code's settings.json
func setVSCodeProxy(proxyServer string, enable bool) error {
	appData := os.Getenv("APPDATA")
	settingsPath := filepath.Join(appData, "Code", "User", "settings.json")

	settings := make(map[string]interface{})
	if _, err := os.Stat(settingsPath); err == nil {
		data, err := os.ReadFile(settingsPath)
		if err == nil && len(data) > 0 {
			_ = json.Unmarshal(data, &settings)
		}
	}

	if enable {
		settings["http.proxy"] = "http://" + proxyServer
	} else {
		if _, exists := settings["http.proxy"]; exists {
			delete(settings, "http.proxy")
		}
	}

	// Write back the updated settings, preserving all other keys
	file, err := os.OpenFile(settingsPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(settings); err != nil {
		return err
	}

	return nil
}

// getDefaultGateway finds the default gateway IP address by parsing the output of the 'route print' command.
func getDefaultGateway() (string, error) {
	// Execute the command to print the IP routing table.
	cmd := exec.Command("route", "print", "0.0.0.0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to execute route command: %v", err)
	}

	// Use a regular expression to find the gateway address for the default route (0.0.0.0).
	re := regexp.MustCompile(`0.0.0.0\s+0.0.0.0\s+(\d+\.\d+\.\d+\.\d+)`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return "", fmt.Errorf("gateway not found")
	}

	return matches[1], nil
}

// setProxySettings modifies the Windows Registry to enable/disable and set the system proxy.
func setProxySettings(proxyServer string, enable int) error {
	// Open the necessary registry key with permissions to set values.
	k, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		registry.QUERY_VALUE|registry.SET_VALUE,
	)
	if err != nil {
		return err
	}
	defer k.Close()

	// Set the proxy server address and port.
	if err := k.SetStringValue("ProxyServer", proxyServer); err != nil {
		return err
	}

	// Enable or disable the proxy.
	if err := k.SetDWordValue("ProxyEnable", uint32(enable)); err != nil {
		return err
	}

	return nil
}

// getCurrentProxy reads the current proxy server setting from the Windows Registry.
func getCurrentProxy() (string, error) {
	// Open the registry key with permissions to query values.
	k, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		registry.QUERY_VALUE,
	)
	if err != nil {
		return "", err
	}
	defer k.Close()

	// Retrieve the "ProxyServer" string value.
	proxyServer, _, err := k.GetStringValue("ProxyServer")
	if err != nil && err != registry.ErrNotExist {
		return "", err
	}

	return proxyServer, nil
}

// updatePowerShellProfile adds or removes proxy environment variables from the user's PowerShell profile.
// This version uses a more robust method of removing the old block before adding a new one.
func updatePowerShellProfile(proxyServer string, enable bool) error {
	// Construct the path to the PowerShell profile.
	profilePath := os.Getenv("USERPROFILE") + "\\Documents\\WindowsPowerShell\\Microsoft.PowerShell_profile.ps1"
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		// Create the profile file if it doesn't exist.
		file, err := os.Create(profilePath)
		if err != nil {
			return err
		}
		file.Close()
	}

	// Read the existing content of the profile.
	fileContent, err := os.ReadFile(profilePath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(fileContent), "\n")
	var newLines []string

	// First, remove any existing proxy block from the lines by filtering it out.
	inBlock := false
	for _, line := range lines {
		// A line that starts with "# Proxy Setting" toggles whether we are in the block.
		if strings.HasPrefix(line, "# Proxy Setting") {
			inBlock = !inBlock
			continue // Skip the marker lines themselves.
		}
		// Only add lines that are not inside a proxy block.
		if !inBlock {
			// Trim carriage returns that can linger on Windows
			newLines = append(newLines, strings.TrimRight(line, "\r"))
		}
	}

	// If enabling the proxy, add the new, correct block to the end.
	if enable {
		proxyBlock := fmt.Sprintf(
			"# Proxy Setting\n$proxy = \"http://%s\"\n$env:HTTP_PROXY = $proxy\n$env:HTTPS_PROXY = $proxy\n[System.Net.WebRequest]::DefaultWebProxy = New-Object System.Net.WebProxy($proxy)\n# Proxy Setting",
			proxyServer,
		)
		// Add a newline for separation if the file isn't empty.
		if len(newLines) > 0 && newLines[len(newLines)-1] != "" {
			newLines = append(newLines, "")
		}
		newLines = append(newLines, proxyBlock)
	}

	// Join the processed lines and write them back to the file.
	newContent := strings.Join(newLines, "\n")
	err = os.WriteFile(profilePath, []byte(newContent), 0644)
	if err != nil {
		return err
	}

	return nil
}

func main() {
	for {
		// Get and display current proxy status
		currentProxy, err := getCurrentProxy()
		if err != nil {
			fmt.Println("Proxy Status: Unknown (could not read settings)")
		} else {
			if currentProxy != "" {
				fmt.Printf("Proxy Status: Active (%s)\n\n", currentProxy)
			} else {
				fmt.Println("Proxy Status: Inactive\n")
			}
		}

		fmt.Println("Select an option:")
		fmt.Println("1. Set/Update Proxy")
		fmt.Println("2. Unset Proxy")
		fmt.Println("3. Exit")
		fmt.Print("Enter your choice: ")

		var choice int
		_, err = fmt.Scanln(&choice)
		if err != nil {
			fmt.Println("Invalid input. Please enter a number.\n")
			// Clear scanner buffer
			var temp string
			fmt.Scanln(&temp)
			continue
		}

		switch choice {
		case 1:
			// Set/Update Proxy
			gateway, err := getDefaultGateway()
			if err != nil {
				fmt.Printf("Error getting gateway: %v\n", err)
				os.Exit(1)
			}
			proxyServer := fmt.Sprintf("%s:10808", gateway)

			if err := setProxySettings(proxyServer, 1); err != nil {
				fmt.Printf("Error setting proxy: %v\n", err)
			}
			if err := updatePowerShellProfile(proxyServer, true); err != nil {
				fmt.Printf("Error updating PowerShell profile: %v\n", err)
			}
			if err := setVSCodeProxy(proxyServer, true); err != nil {
				fmt.Printf("Error updating VS Code proxy: %v\n", err)
			}
			fmt.Printf("Proxy set to %s (system, PowerShell, VS Code)\n", proxyServer)
			fmt.Println("\nIMPORTANT: You must open a new PowerShell window for changes to take effect.")

		case 2:
			// Unset Proxy
			if err := setProxySettings("", 0); err != nil {
				fmt.Printf("Error clearing proxy: %v\n", err)
			}
			if err := updatePowerShellProfile("", false); err != nil {
				fmt.Printf("Error updating PowerShell profile: %v\n", err)
			}
			if err := setVSCodeProxy("", false); err != nil {
				fmt.Printf("Error updating VS Code proxy: %v\n", err)
			}
			fmt.Println("Proxy settings cleared (system, PowerShell, VS Code)")
			fmt.Println("\nIMPORTANT: You must open a new PowerShell window for changes to take effect.")

		case 3:
			fmt.Println("Exiting.")
			return

		default:
			fmt.Println("Invalid choice. Please select a valid option.")
		}
		fmt.Println() // Add a newline for better spacing
	}
}
