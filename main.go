package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"golang.org/x/sys/windows/registry"
)

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
	// Get the default gateway IP.
	gateway, err := getDefaultGateway()
	if err != nil {
		fmt.Printf("Error getting gateway: %v\n", err)
		os.Exit(1)
	}

	// The proxy server for the registry should not have a protocol prefix.
	proxyServer := fmt.Sprintf("%s:10808", gateway)
	currentProxy, err := getCurrentProxy()
	if err != nil {
		fmt.Printf("Error reading proxy settings: %v\n", err)
		os.Exit(1)
	}

	// This logic toggles the proxy. If it's on, it turns it off. If it's off, it turns it on.
	// It also handles updating the proxy if the gateway IP has changed.
	if currentProxy != "" {
		if currentProxy == proxyServer {
			// Current setting matches, so we disable the proxy.
			if err := setProxySettings("", 0); err != nil {
				fmt.Printf("Error clearing proxy: %v\n", err)
				os.Exit(1)
			}
			if err := updatePowerShellProfile("", false); err != nil {
				fmt.Printf("Error updating PowerShell profile: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Proxy settings cleared")
		} else {
			// A different proxy is set, so we update it to the correct one.
			if err := setProxySettings(proxyServer, 1); err != nil {
				fmt.Printf("Error updating proxy: %v\n", err)
				os.Exit(1)
			}
			if err := updatePowerShellProfile(proxyServer, true); err != nil {
				fmt.Printf("Error updating PowerShell profile: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Proxy updated to %s\n", proxyServer)
		}
	} else {
		// No proxy is set, so we enable it.
		if err := setProxySettings(proxyServer, 1); err != nil {
			fmt.Printf("Error setting proxy: %v\n", err)
			os.Exit(1)
		}
		if err := updatePowerShellProfile(proxyServer, true); err != nil {
			fmt.Printf("Error updating PowerShell profile: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Proxy set to %s\n", proxyServer)
	}

	fmt.Println("\nIMPORTANT: You must open a new PowerShell window for changes to take effect.")
}
