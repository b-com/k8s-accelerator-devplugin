package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1beta1"
	"net"
	"os"
	"path"
	"strconv"
	"time"
)

const (
	resourceName = "b-com.com/accelerator"
	serverSock   = pluginapi.DevicePluginPath + "accelerator.sock"
)

// AcceleratorDevicePlugin implements the Kubernetes device plugin API
type AcceleratorDevicePlugin struct {
	devs         []*pluginapi.Device
	socket       string
	accelNbUnits int

	stop   chan interface{}
	health chan *pluginapi.Device

	server *grpc.Server
}

// NewAcceleratorDevicePlugin returns an initialized AcceleratorDevicePlugin
func NewAcceleratorDevicePlugin(accelNbUnits int) *AcceleratorDevicePlugin {
	var devs []*pluginapi.Device
	for id := 0; id < accelNbUnits; id++ {
		devs = append(devs, &pluginapi.Device{
			ID:     fmt.Sprintf("%d", id),
			Health: pluginapi.Healthy,
		})
	}

	return &AcceleratorDevicePlugin{
		socket:       serverSock,
		devs:         devs,
		accelNbUnits: accelNbUnits,
		stop:         make(chan interface{}),
		health:       make(chan *pluginapi.Device),
	}
}

// dial establishes the gRPC communication with the registered device plugin.
func dial(unixSocketPath string, timeout time.Duration) (*grpc.ClientConn, error) {
	c, err := grpc.Dial(unixSocketPath, grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithTimeout(timeout),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
	)

	if err != nil {
		return nil, err
	}

	return c, nil
}

// Start starts the gRPC server of the device plugin
func (m *AcceleratorDevicePlugin) Start() error {
	err := m.cleanup()
	if err != nil {
		return err
	}

	sock, err := net.Listen("unix", m.socket)
	if err != nil {
		return err
	}

	m.server = grpc.NewServer([]grpc.ServerOption{}...)
	pluginapi.RegisterDevicePluginServer(m.server, m)

	go m.server.Serve(sock)

	// Wait for server to start by launching a blocking connection
	conn, err := dial(m.socket, 5*time.Second)
	if err != nil {
		return err
	}
	conn.Close()

	//	go m.healthcheck()

	return nil
}

// Stop stops the gRPC server
func (m *AcceleratorDevicePlugin) Stop() error {
	if m.server == nil {
		return nil
	}

	m.server.Stop()
	m.server = nil
	close(m.stop)
	// sleep needed before socket remove to send devices update on stop ??
	time.Sleep(1 * time.Second)
	return m.cleanup()
}

// Register registers the device plugin for the given resourceName with Kubelet.
func (m *AcceleratorDevicePlugin) Register(kubeletEndpoint, resourceName string) error {
	conn, err := dial(kubeletEndpoint, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pluginapi.NewRegistrationClient(conn)
	reqt := &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     path.Base(m.socket),
		ResourceName: resourceName,
	}

	_, err = client.Register(context.Background(), reqt)
	if err != nil {
		return err
	}
	return nil
}

// ListAndWatch lists devices and update that list according to the health status
func (m *AcceleratorDevicePlugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
	//   for _, dev := range m.devs {
	//      log.Debugf("dev %s id %s", dev.ID, dev.Health)
	//   }
	s.Send(&pluginapi.ListAndWatchResponse{Devices: m.devs})

	for {
		select {
		case <-m.stop:
			log.Debugf("ListAndWatch stop: mark all devices unhealthy")
			for _, dev := range m.devs {
				dev.Health = pluginapi.Unhealthy
			}
			// marking all devs unhealthy does not change node capacity and allocatable ??
			s.Send(&pluginapi.ListAndWatchResponse{Devices: m.devs})
			return nil
		case d := <-m.health:
			d.Health = pluginapi.Unhealthy
			s.Send(&pluginapi.ListAndWatchResponse{Devices: m.devs})
		}
	}
}

func (m *AcceleratorDevicePlugin) unhealthy(dev *pluginapi.Device) {
	m.health <- dev
}

// Allocate which return list of devices.
func (m *AcceleratorDevicePlugin) Allocate(ctx context.Context, r *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {

	var deviceIDs string
	if len(r.ContainerRequests) > 0 {
		for _, req := range r.ContainerRequests {
			log.Debugf("Request IDs: %v", req.DevicesIDs)
			for _, idstr := range req.DevicesIDs {
				id, _ := strconv.Atoi(idstr)
				if (id < 0) || (id >= m.accelNbUnits) {
					return nil, fmt.Errorf("invalid allocation request: unknown device: %s", idstr)
				}
				if len(deviceIDs) > 0 {
					deviceIDs = deviceIDs + "," + idstr
				} else {
					deviceIDs = idstr
				}
			}
		}
	}
	log.Infof("Allocate IDs: %s", deviceIDs)
	response := pluginapi.AllocateResponse{}
	containerResponse := pluginapi.ContainerAllocateResponse{
		Envs: map[string]string{
			"ACCELERATOR_UNITS": deviceIDs,
		},
	}
	response.ContainerResponses = []*pluginapi.ContainerAllocateResponse{&containerResponse}
	return &response, nil
}

// GetDevicePluginOptions returns options to be communicated with Device Manager
func (m *AcceleratorDevicePlugin) GetDevicePluginOptions(ctx context.Context, e *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return nil, nil
}

// PreStartContainer is called, if indicated by Device Plugin during registeration phase, before each container start
func (m *AcceleratorDevicePlugin) PreStartContainer(context.Context, *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return nil, nil
}

func (m *AcceleratorDevicePlugin) cleanup() error {
	if err := os.Remove(m.socket); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

//func (m *AcceleratorDevicePlugin) healthcheck() {
//	ctx, cancel := context.WithCancel(context.Background())
//
//	xids := make(chan *pluginapi.Device)
//	go watchXIDs(ctx, m.devs, xids)
//
//	for {
//		select {
//		case <-m.stop:
//			cancel()
//			return
//		case dev := <-xids:
//			m.unhealthy(dev)
//		}
//	}
//}

// Serve starts the gRPC server and register the device plugin to Kubelet
func (m *AcceleratorDevicePlugin) Serve() error {
	err := m.Start()
	if err != nil {
		log.Errorf("Could not start device plugin: %v", err)
		return err
	}
	log.Infof("Starting to serve on %s", m.socket)

	err = m.Register(pluginapi.KubeletSocket, resourceName)
	if err != nil {
		log.Errorf("Could not register device plugin: %v", err)
		m.Stop()
		return err
	}
	log.Infof("Registered device plugin with Kubelet")

	return nil
}
