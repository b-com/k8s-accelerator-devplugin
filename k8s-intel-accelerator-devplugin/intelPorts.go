package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	sysfsFpgaClassPath = "/sys/class/fpga"
	osdevPath          = "/dev"
	portDeviceFmt      = "intel-fpga-port.%d"
)

type pciBdf struct {
	bus      int
	device   int
	function int
}

func (bdf *pciBdf) label() string {
	return fmt.Sprintf("%02d:%02d.%d", bdf.bus, bdf.device, bdf.function)
}

type intelPort struct {
	instanceId int
	name       string
	bdf        pciBdf
}

type intelPorts struct {
	portList []intelPort
}

func busdevfnFromSymlink(linkname string) pciBdf {
	symlink, err := filepath.EvalSymlinks(linkname)
	if err == nil {
		r := regexp.MustCompile("[^/:.]+")
		items := r.FindAllString(symlink, -1)
		bus, _ := strconv.Atoi(items[len(items)-3])
		device, _ := strconv.Atoi(items[len(items)-2])
		function, _ := strconv.Atoi(items[len(items)-1])
		return pciBdf{bus, device, function}
	}

	log.Errorf("Failed to get busdevfn from " + linkname)
	return pciBdf{}
}

func (ires *intelPorts) enumerate() {
	var bdf pciBdf

	ires.portList = []intelPort{}
	entries, err := ioutil.ReadDir(sysfsFpgaClassPath)
	if err != nil {
		log.Errorf("[IntelPorts] Failed to read directory content from " + sysfsFpgaClassPath + ": " + err.Error())
		return
	}

	for _, f := range entries {
		// Get instance ID from entry name "intel-fpga-dev.<instance id>"
		substrs := strings.Split(f.Name(), ".")
		instanceId, _ := strconv.Atoi(substrs[len(substrs)-1])

		syspath := filepath.Join(sysfsFpgaClassPath, f.Name())
		portname := fmt.Sprintf(portDeviceFmt, instanceId)
		if _, err := os.Stat(filepath.Join(syspath, portname)); err == nil {
			// Get bdf (bus, device, function) from symlink "device -> ../../../0000:06:00.0"
			bdf = busdevfnFromSymlink(filepath.Join(syspath, "device"))
			// Add intel port to list
			port := intelPort{name: portname, instanceId: instanceId, bdf: bdf}
			ires.portList = append(ires.portList, port)
			log.Infof("[IntelPorts] new AFU PORT: name %s, instance %d, bdf %s", port.name, port.instanceId, port.bdf.label())
		}
	}

	log.Infof("[IntelPorts] Nb AFU ports %d", len(ires.portList))
	return
}
