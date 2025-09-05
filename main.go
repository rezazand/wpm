package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// Environment constants
const (
	ENV_SYSTEM     = 1 << iota // 1
	ENV_POWERSHELL            // 2
	ENV_VSCODE                // 4
	ENV_NPM                   // 8
)

var envNames = map[int]string{
	ENV_SYSTEM:     "System Registry",
	ENV_POWERSHELL: "PowerShell Profile",
	ENV_VSCODE:     "VS Code",
	ENV_NPM:        "npm",
}

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

// setNpmProxy sets or clears the proxy in the .npmrc file
func setNpmProxy(proxyServer string, enable bool) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not get user home directory: %v", err)
	}
	
	npmrcPath := filepath.Join(homeDir, ".npmrc")
	
	var lines []string
	
	// Read existing .npmrc if it exists
	if _, err := os.Stat(npmrcPath); err == nil {
		content, err := os.ReadFile(npmrcPath)
		if err != nil {
			return fmt.Errorf("could not read .npmrc: %v", err)
		}
		lines = strings.Split(string(content), "\n")
	}
	
	// Remove existing proxy lines
	var newLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "proxy=") && 
		   trimmed != "" {
			newLines = append(newLines, strings.TrimRight(line, "\r"))
		}
	}
	
	// Add proxy lines if enabling
	if enable {
		newLines = append(newLines, fmt.Sprintf("proxy=http://%s", proxyServer))
	}
	
	// Write back to .npmrc
	content := strings.Join(newLines, "\n")
	if len(newLines) > 0 {
		content += "\n"
	}
	
	err = os.WriteFile(npmrcPath, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("could not write .npmrc: %v", err)
	}
	
	return nil
}

// parseEnvironmentSelection parses user input for environment selection
func parseEnvironmentSelection(input string) int {
	input = strings.TrimSpace(input)
	if input == "" || strings.ToLower(input) == "a" {
		return ENV_SYSTEM | ENV_POWERSHELL | ENV_VSCODE | ENV_NPM // All environments
	}

	selectedEnvs := 0
	for _, char := range input {
		switch char {
		case '1':
			selectedEnvs |= ENV_SYSTEM
		case '2':
			selectedEnvs |= ENV_POWERSHELL
		case '3':
			selectedEnvs |= ENV_VSCODE
		case '4':
			selectedEnvs |= ENV_NPM
		}
	}

	return selectedEnvs
}

// displaySelectedEnvironments shows which environments are selected
func displaySelectedEnvironments(envMask int) {
	if envMask == 0 {
		fmt.Println("No environments selected.")
		return
	}

	fmt.Print("Selected environments: ")
	var selected []string
	
	if envMask&ENV_SYSTEM != 0 {
		selected = append(selected, envNames[ENV_SYSTEM])
	}
	if envMask&ENV_POWERSHELL != 0 {
		selected = append(selected, envNames[ENV_POWERSHELL])
	}
	if envMask&ENV_VSCODE != 0 {
		selected = append(selected, envNames[ENV_VSCODE])
	}
	if envMask&ENV_NPM != 0 {
		selected = append(selected, envNames[ENV_NPM])
	}
	
	fmt.Println(strings.Join(selected, ", "))
}

// applyProxySettings applies proxy settings to selected environments
func applyProxySettings(proxyServer string, enable bool, envMask int) {
	var errors []string
	var success []string

	if envMask&ENV_SYSTEM != 0 {
		var enableInt int
		if enable {
			enableInt = 1
		}
		if err := setProxySettings(proxyServer, enableInt); err != nil {
			errors = append(errors, fmt.Sprintf("System Registry: %v", err))
		} else {
			success = append(success, "System Registry")
		}
	}

	if envMask&ENV_POWERSHELL != 0 {
		if err := updatePowerShellProfile(proxyServer, enable); err != nil {
			errors = append(errors, fmt.Sprintf("PowerShell Profile: %v", err))
		} else {
			success = append(success, "PowerShell Profile")
		}
	}

	if envMask&ENV_VSCODE != 0 {
		if err := setVSCodeProxy(proxyServer, enable); err != nil {
			errors = append(errors, fmt.Sprintf("VS Code: %v", err))
		} else {
			success = append(success, "VS Code")
		}
	}

	if envMask&ENV_NPM != 0 {
		if err := setNpmProxy(proxyServer, enable); err != nil {
			errors = append(errors, fmt.Sprintf("npm: %v", err))
		} else {
			success = append(success, "npm")
		}
	}

	// Display results
	if len(success) > 0 {
		action := "set"
		if !enable {
			action = "cleared"
		}
		fmt.Printf("✓ Proxy %s successfully for: %s\n", action, strings.Join(success, ", "))
	}

	if len(errors) > 0 {
		fmt.Println("\n⚠ Errors occurred:")
		for _, err := range errors {
			fmt.Printf("  - %s\n", err)
		}
	}

	if envMask&ENV_POWERSHELL != 0 {
		fmt.Println("\nIMPORTANT: You must open a new PowerShell window for changes to take effect.")
	}
}

func main() {
    for {
        // Get and display current proxy status
        currentProxy, err := getCurrentProxy()
        gateway, gwErr := getDefaultGateway() // Get gateway to determine tag

        if err != nil {
            fmt.Println("Proxy Status: Unknown (could not read settings)")
        } else {
            if currentProxy != "" {
                var tag string
                // Extract IP from "IP:port"
                proxyIP := strings.Split(currentProxy, ":")[0]

                if gwErr == nil && proxyIP == gateway {
                    tag = " (Gateway)"
                } else if proxyIP == "127.0.0.1" {
                    tag = " (Localhost)"
                } else {
                    tag = " (Custom)"
                }
                fmt.Printf("Proxy Status: Active (%s)%s\n\n", currentProxy, tag)
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
            var proxyServer string
            // Use the gateway fetched earlier
            if gwErr != nil {
                fmt.Printf("Warning: Could not determine default gateway: %v\n", gwErr)
                gateway = "unavailable"
            }

            fmt.Println("\nSelect proxy configuration:")
            fmt.Printf("1. Default Gateway (%s:10808)\n", gateway)
            fmt.Println("2. Localhost (127.0.0.1:10808)")
            fmt.Println("3. Custom IP:Port")
            fmt.Println("4. Back to main menu")
            fmt.Print("Enter your choice: ")

            var proxyChoice int
            _, err = fmt.Scanln(&proxyChoice)
            if err != nil {
                fmt.Println("Invalid input. Please enter a number.\n")
                continue
            }

            switch proxyChoice {
            case 1:
                if gateway == "unavailable" {
                    fmt.Println("Cannot use default gateway as it could not be determined. Please choose another option.")
                    continue
                }
                proxyServer = fmt.Sprintf("%s:10808", gateway)
            case 2:
                proxyServer = "127.0.0.1:10808"
            case 3:
                fmt.Print("Enter custom IP:Port (e.g., 192.168.1.1:8080): ")
               	_, err := fmt.Scanln(&proxyServer)
                if err != nil || proxyServer == "" {
                    fmt.Println("Invalid input.")
                    continue
                }
            case 4:
                continue // Go back to the main menu
            default:
                fmt.Println("Invalid choice. Please select a valid option.")
                continue
            }

            // Environment Selection
            fmt.Println("\n🎯 Select environments to configure:")
            fmt.Println("1. System Registry")
            fmt.Println("2. PowerShell Profile") 
            fmt.Println("3. VS Code")
            fmt.Println("4. npm")
            fmt.Println("\n📝 Input options:")
            fmt.Println("- Press ENTER or type 'A' for ALL environments")
            fmt.Println("- Type numbers for specific environments (e.g., '13' for System + VS Code, '24' for PowerShell + npm)")
            fmt.Print("\nYour selection: ")

            var envInput string
            fmt.Scanln(&envInput)
            
            selectedEnvs := parseEnvironmentSelection(envInput)
            if selectedEnvs == 0 {
                fmt.Println("No valid environments selected. Please try again.")
                continue
            }

            fmt.Println()
            displaySelectedEnvironments(selectedEnvs)
            fmt.Printf("Setting proxy to: %s\n", proxyServer)
            
            applyProxySettings(proxyServer, true, selectedEnvs)

        case 2:
            // Unset Proxy
            fmt.Println("\n🎯 Select environments to clear proxy from:")
            fmt.Println("1. System Registry")
            fmt.Println("2. PowerShell Profile")
            fmt.Println("3. VS Code") 
            fmt.Println("4. npm")
            fmt.Println("\n📝 Input options:")
            fmt.Println("- Press ENTER or type 'A' for ALL environments")
            fmt.Println("- Type numbers for specific environments (e.g., '13' for System + VS Code, '24' for PowerShell + npm)")
            fmt.Print("\nYour selection: ")

            var envInput string
            fmt.Scanln(&envInput)
            
            selectedEnvs := parseEnvironmentSelection(envInput)
            if selectedEnvs == 0 {
                fmt.Println("No valid environments selected. Please try again.")
                continue
            }

            fmt.Println()
            displaySelectedEnvironments(selectedEnvs)
            fmt.Println("Clearing proxy settings...")
            
            applyProxySettings("", false, selectedEnvs)

        case 3:
            fmt.Println("Exiting.")
            return

        default:
            fmt.Println("Invalid choice. Please select a valid option.")
        }
        fmt.Println() // Add a newline for better spacing
    }
}
