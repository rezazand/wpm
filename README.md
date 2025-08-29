# WPM (Windows Proxy Manager)

## Introduction
A lightweight and efficient tool for managing proxy settings on Windows. This tool provides seamless proxy configuration for the system, PowerShell, and VS Code with an intuitive interface.

## Features
- 🔧 System-wide proxy configuration
- 💻 PowerShell proxy settings
- 🔌 VS Code proxy integration
- 🖼️ Custom icon support
- ⚡ Lightweight and fast execution

## Prerequisites
- Go 1.22 or later
- Windows operating system
- Administrator privileges (for system-wide changes)

## Installation

### Option 1: Download Release
Download the latest release from the [Releases](../../releases) page.

### Option 2: Build from Source

#### Basic Build
```bash
git clone <repository-url>
cd proxy_manager
go build
```

#### Building with Custom Icon

To build the project with a custom icon:

1. **Install the `rsrc` tool:**
    ```bash
    go install github.com/akavel/rsrc@latest
    ```

2. **Generate the resource file:**
    ```bash
    rsrc -ico your_icon_name.ico
    ```
    > This creates a `rsrc.syso` file in your project directory.

3. **Build the project:**
    ```bash
    go build -ldflags="-s -w"
    ```

The generated executable will include your custom icon and be optimized for size.

## Usage
Run the executable with administrator privileges for full functionality:
```cmd
WPM.exe
```

## License
This project is licensed under the MIT License - see the LICENSE file for details.