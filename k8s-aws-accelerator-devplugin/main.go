package main

import (
	"flag"
	log "github.com/Sirupsen/logrus"
	"github.com/fsnotify/fsnotify"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1beta1"
	"os"
	"syscall"
)

const (
	syslogFile = "/var/log/k8s-accelerator-devplugin.log"
)

var (
	logLevel     = flag.String("log-level", "info", "Define the logging level: error, info, debug.")
	logfatal     *log.Logger
	accelNbUnits = flag.Int("accel-nbunits", 40, "Total nb accelerator units of FPGA")
)

func loginit() {
	f, err := os.OpenFile(syslogFile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		// Cannot open log file. Logging to stderr
		log.Errorln("Log to file failed:", err)
	} else {
		log.SetOutput(f)
	}

	Formatter := new(log.TextFormatter)
	Formatter.TimestampFormat = "02-01-2006 15:04:05"
	Formatter.FullTimestamp = true
	log.SetFormatter(Formatter)
	switch *logLevel {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	}

	// Fatal errors are logged to stderr, followed by exit()
	logfatal = log.New()
}

func main() {
	flag.Parse()
	loginit()

	if *accelNbUnits == 0 {
		log.Errorf("No accelerator device units found.")
		return
	}
	log.Infof("FPGA contains %d accelerator device units", *accelNbUnits)

	log.Infof("Starting FS watcher.")
	watcher, err := newFSWatcher(pluginapi.DevicePluginPath)
	if err != nil {
		log.Errorf("Failed to created FS watcher.alpha")
		os.Exit(1)
	}
	defer watcher.Close()

	log.Infof("Starting OS watcher.")
	sigs := newOSWatcher(syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	restart := true
	var devicePlugin *AcceleratorDevicePlugin

L:
	for {
		if restart {
			if devicePlugin != nil {
				log.Debugf("restart: first stop device plugin")
				devicePlugin.Stop()
			}

			devicePlugin = NewAcceleratorDevicePlugin(*accelNbUnits)
			if err := devicePlugin.Serve(); err != nil {
				log.Errorf("Could not contact Kubelet, retrying. Did you enable the device plugin feature gate?")
			} else {
				restart = false
			}
		}

		select {
		case event := <-watcher.Events:
			if event.Name == pluginapi.KubeletSocket && event.Op&fsnotify.Create == fsnotify.Create {
				log.Infof("inotify: %s created, restarting.", pluginapi.KubeletSocket)
				restart = true
			}

		case err := <-watcher.Errors:
			log.Errorf("inotify: %s", err)

		case s := <-sigs:
			switch s {
			case syscall.SIGHUP:
				log.Infof("Received SIGHUP, restarting.")
				restart = true
			default:
				log.Infof("Received signal \"%v\", shutting down.", s)
				devicePlugin.Stop()
				break L
			}
		}
	}
}
