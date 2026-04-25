# Framework LED Matrix System Monitor

A lightweight system monitor written in Go, specifically designed for the **Framework Laptop 16** equipped with the **LED Matrix module** and an **NVIDIA GPU**.

This tool utilizes the side-mounted LED matrices to display real-time system metrics, providing a visual at-a-glance dashboard of your laptop's performance.

## 🎯 Target Hardware
- **Laptop:** Framework Laptop 16
- **Module:** LED Matrix (Expansion Bay)
- **GPU:** NVIDIA (Requires NVML/NVIDIA drivers installed)
- **OS:** Linux (Tested on Fedora/Ubuntu)

## ✨ Features
- **Real-time Monitoring:** Displays three vertical bars on the LED matrix:
  1. **CPU Usage** (Left bar)
  2. **RAM Usage** (Middle bar)
  3. **VRAM Usage** (Right bar - NVIDIA only)
- **Auto-Brightness:** Uses the laptop's built-in **Ambient Light Sensor (ALS)** to automatically adjust matrix brightness based on your environment.
- **Daemon Mode:** Can run in the background as a systemd service or a detached daemon.
- **Low Overhead:** Written in Go with minimal resource footprint.

## 🚀 Getting Started

### Prerequisites
- Go 1.21 or later.
- NVIDIA drivers and `libnvidia-ml.so` (NVML) for GPU monitoring.
- Permission to access `/dev/ttyACM*` (usually part of the `dialout` or `uucp` group).

### Installation
1. Clone the repository:
   ```bash
   git clone git@github.com:jtl5770/ledmatrix.git
   cd ledmatrix
   ```
2. Build the project:
   ```bash
   go build -o ledmatrix
   ```

### Usage
Run the monitor directly:
```bash
./ledmatrix
```

**Command-line Options:**
- `-d`: Run as a daemon (logs to syslog).
- `-a`: Enable auto-brightness (default: true).
- `-b <1-255>`: Set a static brightness and disable auto-brightness.

### Systemd Service
To run this automatically on login, you can use the provided service file:

1. Copy the binary to your local bin:
   ```bash
   mkdir -p ~/bin
   cp ledmatrix ~/bin/
   ```
2. Copy and enable the service:
   ```bash
   mkdir -p ~/.config/systemd/user/
   cp ledmatrix.service ~/.config/systemd/user/
   systemctl --user daemon-reload
   systemctl --user enable --now ledmatrix.service
   ```
*Note: You may need to adjust the paths in `ledmatrix.service` if your home directory is different.*

## 🛠️ Technical Details
- **Communication:** Communicates with the LED Matrix via Serial over USB (`/dev/ttyACM?`).
- **Sensors:** 
  - CPU/RAM via `gopsutil`.
  - VRAM via `go-nvml`.
  - Brightness via IIO (`/sys/bus/iio/devices/iio:device*`).

## ⚖️ License
GPL-3.0-or-later
