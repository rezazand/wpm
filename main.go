package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"golang.org/x/sys/windows/registry"
)

func getDefaultGateway() (string, error) {
	cmd := exec.Command("route", "print", "0.0.0.0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to execute route command: %v", err)
	}

	re := regexp.MustCompile(`0.0.0.0\s+0.0.0.0\s+(\d+\.\d+\.\d+\.\d+)`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return "", fmt.Errorf("gateway not found")
	}

	return matches[1], nil
}

func setProxySettings(proxyServer string, enable int) error {
	k, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		registry.QUERY_VALUE|registry.SET_VALUE,
	)
	if err != nil {
		return err
	}
	defer k.Close()

	if err := k.SetStringValue("ProxyServer", proxyServer); err != nil {
		return err
	}

	if err := k.SetDWordValue("ProxyEnable", uint32(enable)); err != nil {
		return err
	}

	return nil
}

func getCurrentProxy() (string, error) {
	k, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		registry.QUERY_VALUE,
	)
	if err != nil {
		return "", err
	}
	defer k.Close()

	proxyServer, _, err := k.GetStringValue("ProxyServer")
	if err != nil && err != registry.ErrNotExist {
		return "", err
	}

	return proxyServer, nil
}

func main() {
	gateway, err := getDefaultGateway()
	if err != nil {
		fmt.Printf("Error getting gateway: %v\n", err)
		os.Exit(1)
	}

	proxyServer := fmt.Sprintf("%s:10808", gateway)
	currentProxy, err := getCurrentProxy()
	if err != nil {
		fmt.Printf("Error reading proxy settings: %v\n", err)
		os.Exit(1)
	}

	if currentProxy != "" {
		if currentProxy == proxyServer {
			// Disable proxy
			if err := setProxySettings("", 0); err != nil {
				fmt.Printf("Error clearing proxy: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Proxy settings cleared")
		} else {
			// Update proxy
			if err := setProxySettings(proxyServer, 1); err != nil {
				fmt.Printf("Error updating proxy: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Proxy updated to %s\n", proxyServer)
		}
	} else {
		// Enable proxy
		if err := setProxySettings(proxyServer, 1); err != nil {
			fmt.Printf("Error setting proxy: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Proxy set to %s\n", proxyServer)
	}

	fmt.Println("You might need to restart applications or refresh network settings for changes to take effect")
}