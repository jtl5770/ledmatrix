package main

import (
	"flag"
	"fmt"
	"log"
	"log/syslog"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/sevlyar/go-daemon"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

func getPIDFilePath() string {
	// Standard location for user-level daemons
	if runDir := os.Getenv("XDG_RUNTIME_DIR"); runDir != "" {
		return filepath.Join(runDir, "ledmatrix.pid")
	}
	// Fallback for system-level or if XDG_RUNTIME_DIR is missing
	if os.Getuid() == 0 {
		return "/run/ledmatrix.pid"
	}
	// Final fallback to a safe user-writable location
	return filepath.Join(os.TempDir(), "ledmatrix.pid")
}

func initNVML() nvml.Device {
	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		log.Printf("Failed to initialize NVML: %v", nvml.ErrorString(ret))
		return nil
	}
	log.Println("NVML initialized successfully.")

	device, ret := nvml.DeviceGetHandleByIndex(0)
	if ret != nvml.SUCCESS {
		log.Printf("Failed to get GPU handle: %v", nvml.ErrorString(ret))
		return nil
	}
	return device
}

func getVRAMPercent(device nvml.Device) (float64, error) {
	if device == nil {
		return 0.0, fmt.Errorf("no GPU device")
	}
	info, ret := device.GetMemoryInfo()
	if ret != nvml.SUCCESS {
		return 0.0, fmt.Errorf("NVML error: %v", nvml.ErrorString(ret))
	}
	if info.Total == 0 {
		return 0.0, nil
	}
	return (float64(info.Used) / float64(info.Total)) * 100.0, nil
}

func drawBar(m *Matrix, startX, width int, percent float64) {
	percent = math.Max(0.0, math.Min(100.0, percent))
	ledsOn := int((percent / 100.0) * float64(MatrixHeight))

	for x := startX; x < startX+width; x++ {
		bottomY := MatrixHeight - 1
		topY := bottomY - ledsOn
		for y := bottomY; y > topY; y-- {
			// Set individual LED values to max; let Global Brightness (ALS) dim them.
			brightness := 255
			m.SetMatrix(x, y, &brightness)
		}
	}
}

func findMatrixDevice() (string, error) {
	devices, err := filepath.Glob("/dev/ttyACM?")
	if err != nil {
		return "", err
	}
	if len(devices) == 0 {
		return "", fmt.Errorf("no LED Matrix device found in /dev/")
	}
	return devices[0], nil
}

func main() {
	daemonMode := flag.Bool("d", false, "run as daemon")
	autoBrightness := flag.Bool("a", true, "enable auto-brightness using ambient light sensor")
	staticBrightness := flag.Int("b", -1, "set static brightness (1-255) and disable auto-brightness")
	flag.Parse()

	// If -b is provided, it overrides and disables auto-brightness
	if *staticBrightness != -1 {
		*autoBrightness = false
	}

	if *daemonMode {
		pidFile := getPIDFilePath()
		cntxt := &daemon.Context{
			PidFileName: pidFile,
			PidFilePerm: 0o644,
			LogFileName: "", // We will use syslog instead
			WorkDir:     "./",
			Umask:       0o27,
			Args:        []string{"[ledmatrix]"},
		}

		d, err := cntxt.Reborn()
		if err != nil {
			log.Fatal("Unable to run: ", err)
		}
		if d != nil {
			return
		}
		defer cntxt.Release()

		// Initialize Syslog
		sl, err := syslog.New(syslog.LOG_INFO|syslog.LOG_DAEMON, "ledmatrix")
		if err != nil {
			log.Fatal("Unable to initialize syslog: ", err)
		}
		log.SetOutput(sl)
		log.SetFlags(0) // Syslog handles timestamping

		log.Println("Daemon started and logging to syslog")
		log.Printf("PID file: %s", pidFile)
	}

	log.Println("Starting System Monitor for Framework LED Matrix (Go Port)...")

	// Initialize Matrix
	// Use staticBrightness if provided, otherwise default to 128
	initialBrightness := 128
	if *staticBrightness != -1 {
		initialBrightness = *staticBrightness
	}

	device, err := findMatrixDevice()
	if err != nil {
		log.Fatalf("Could not find LED Matrix device: %v", err)
	}
	matrix, err := NewMatrix(initialBrightness, device)
	if err != nil {
		log.Fatalf("Could not connect to LED Matrix: %v", err)
	}
	defer matrix.Close()
	log.Println("Successfully connected to the LED Matrix.")

	// Initialize Ambient Light Sensor
	var als *AmbientLightSensor
	if *autoBrightness {
		als, err = NewAmbientLightSensor()
		if err != nil {
			log.Printf("Warning: Could not initialize ambient light sensor: %v. Auto-brightness disabled.", err)
			*autoBrightness = false
		} else {
			log.Println("Ambient light sensor initialized.")
		}
	}

	// Initialize GPU Monitoring
	gpuDevice := initNVML()
	if gpuDevice != nil {
		defer nvml.Shutdown()
	}

	// Prime CPU percent
	cpu.Percent(0, false)

	// Setup signal handling for clean shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-sigs:
			log.Println("Shutting down cleanly...")
			matrix.Reset(0)
			matrix.CSend()
			return
		case <-ticker.C:
			// 1. Gather System Metrics
			cpuPercents, _ := cpu.Percent(0, false)
			cpuPct := 0.0
			if len(cpuPercents) > 0 {
				cpuPct = cpuPercents[0]
			}

			vmem, _ := mem.VirtualMemory()
			ramPct := vmem.UsedPercent

			vramPct := 0.0
			if gpuDevice != nil {
				var err error
				vramPct, err = getVRAMPercent(gpuDevice)
				if err != nil {
					log.Printf("Error reading VRAM: %v. Invalidating GPU handle.", err)
					nvml.Shutdown()
					gpuDevice = nil
				}
			} else {
				// Try to re-init NVML if it wasn't available
				gpuDevice = initNVML()
			}

			// 2. Adjust Brightness
			if *autoBrightness {
				if als == nil {
					als, _ = NewAmbientLightSensor()
				}
				if als != nil {
					raw, err := als.ReadRaw()
					if err == nil {
						brightness := als.MapToBrightness(raw)
						matrix.SetBrightness(brightness)
					} else {
						log.Printf("Error reading ALS: %v. Invalidating sensor.", err)
						als = nil // Try to re-init on next tick
					}
				}
			}

			// 3. Clear buffer
			matrix.Reset(0)

			// 4. Draw the bars
			drawBar(matrix, 0, 3, cpuPct)
			drawBar(matrix, 3, 3, ramPct)
			drawBar(matrix, 6, 3, vramPct)

			// 5. Send to hardware
			if err := matrix.CSend(); err != nil {
				log.Printf("Error sending to matrix: %v. Attempting to reconnect...", err)
				
				// Try to find the device again as it might have changed path (e.g. ACM0 -> ACM1)
				if newDevice, err := findMatrixDevice(); err == nil {
					if err := matrix.Reopen(newDevice); err == nil {
						log.Printf("Successfully reconnected to matrix on %s", newDevice)
						// Retry sending once after reconnection
						if err := matrix.CSend(); err != nil {
							log.Printf("Failed to send even after reconnection: %v", err)
						}
					} else {
						log.Printf("Failed to reopen matrix device: %v", err)
					}
				} else {
					log.Printf("Could not find matrix device to reconnect: %v", err)
				}
			}
		}
	}
}
