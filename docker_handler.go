package main

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/prometheus/common/model"
	"sync"
	"time"
)

type dockerHandler struct {
	config             envConfig
	client             *client.Client
	writer             *promHandler
	managedContainers  map[string]containerScrapeConfig
	lock               sync.Locker
	startContainerChan chan []string
	stopContainerChan  chan []string
}

const (
	dockerEventActionStart   = "start"
	dockerEventActionDie     = "die"
	dockerEventTypeContainer = "container"

	prometheusEnableScrapeAnnotation      = "prometheus.io/scrape"
	prometheusEnableScrapeAnnotationValue = "true"
	prometheusPortAnnotation              = "prometheus.io/port"
	prometheusIpAnnotation                = "prometheus.io/ip"
	prometheusPathAnnotation              = "prometheus.io/path"
	prometheusSchemeAnnotation            = "prometheus.io/scheme"
	prometheusExtraLabelsAnnotation       = "prometheus.io/extra-labels" // comma separated extra labels for this pod
)

type containerScrapeConfig struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

func newDockerHandler(config envConfig, writer *promHandler) (*dockerHandler, error) {

	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	_, err = dockerClient.Ping(context.Background())
	if err != nil {
		log.Debugf("Got an error while pinging the docker engine %v, err = %v", dockerClient.DaemonHost(), err)
		return nil, err
	}

	networks, err := dockerClient.NetworkList(context.Background(), types.NetworkListOptions{})
	if err != nil {
		log.Debugf("Got an error while listing the networks, err = %v", err)
		return nil, err
	}
	foundNetwork := false
	for _, n := range networks {
		if n.Name == config.DockerNetworkName {
			foundNetwork = true
		}
	}
	if !foundNetwork {
		log.Debugf("No network found with the name %s...", config.DockerNetworkName)
		return nil, fmt.Errorf("no network named %s found. Please docker network create %s", config.DockerNetworkName, config.DockerNetworkName)
	}

	handler := &dockerHandler{
		config:             config,
		client:             dockerClient,
		writer:             writer,
		managedContainers:  make(map[string]containerScrapeConfig),
		lock:               &sync.Mutex{},
		startContainerChan: make(chan []string),
		stopContainerChan:  make(chan []string),
	}

	return handler, nil
}

func (h *dockerHandler) handle() error {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go h.listenToDockerEvents(ctx)
	go h.listDockerContainers(ctx)

	for {
		select {
		case <-ctx.Done():
			return nil
		case containersToAdd := <-h.startContainerChan:
			h.addContainersToScrapeConfig(containersToAdd)
		case containerToRemove := <-h.stopContainerChan:
			h.removeContainersFromScrapeConfig(containerToRemove)
		}
	}
}

func (h *dockerHandler) pollDockerContainers(ctx context.Context) {
	h.listDockerContainers(ctx)
	if h.config.DockerPollFrequency > 0 {
		for range time.Tick(h.config.DockerPollFrequency) {
			h.listDockerContainers(ctx)
		}
	}
}

func (h *dockerHandler) listDockerContainers(ctx context.Context) {

	f := filters.NewArgs(filters.KeyValuePair{Key: "status", Value: "running"},
		filters.KeyValuePair{Key: "label", Value: prometheusEnableScrapeAnnotation + "=" + prometheusEnableScrapeAnnotationValue})
	containers, err := h.client.ContainerList(context.Background(), types.ContainerListOptions{
		Filters: f,
	})
	if err != nil {
		log.Warnf("Got an error while listing the containers, err = %v", err)
		return
	}
	containerIds := make([]string, 0, 0)
	for _, c := range containers {
		if containerIsManaged(c.Labels) {
			log.Debugf("Container %s is managed", c.ID)
			containerIds = append(containerIds, c.ID)
		}
	}

	if len(containerIds) > 0 {
		h.startContainerChan <- containerIds
	}
}

func (h *dockerHandler) listenToDockerEvents(ctx context.Context) {

	ctx, cancel := context.WithCancel(context.Background())
	eventsChan, errsChan := h.client.Events(ctx, types.EventsOptions{})

	defer func() {
		cancel()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-eventsChan:
			log.Tracef("Got an event %v", event)
			if event.Type == dockerEventTypeContainer && event.Action == dockerEventActionStart {
				log.Infof("Got a start of container %s", event.ID)
				h.startContainerChan <- []string{event.ID}
			} else if event.Type == dockerEventTypeContainer && event.Action == dockerEventActionDie {
				log.Infof("Got a stop of container %s", event.ID)
				h.stopContainerChan <- []string{event.ID}
			}
		case err := <-errsChan:
			log.Fatalf("Got an error while listening to events, err = %v", err)
			return
		}
	}
}

func (h *dockerHandler) addContainersToScrapeConfig(containerIds []string) {

	log.Debugf("addContainersToScrapeConfig for %v", containerIds)

	h.lock.Lock()
	defer h.lock.Unlock()

	hasContainersAdded := false

	for _, containerId := range containerIds {

		managed, containerScrapeConfig, err := h.findHostPortAndPathForContainer(containerId)
		if err != nil {
			log.Warnf("Got an error while getting container %s configuration details, err = %v", containerId, err)
		} else if managed {
			_, exists := h.managedContainers[containerId]
			if !exists {
				h.managedContainers[containerId] = containerScrapeConfig
				hasContainersAdded = true
			}
		} else {
			log.Debugf("Container %s is not managed by us, ignoring", containerId)
		}
	}

	if hasContainersAdded {
		err := h.writer.write(h.managedContainers)
		if err != nil {
			log.Warnf("Got an error while trying to write the config to the prometheus file, err = %v", err)
		}
	}
}

func (h *dockerHandler) removeContainersFromScrapeConfig(containerIds []string) {

	log.Debugf("removeContainersFromScrapeConfig for %v", containerIds)

	h.lock.Lock()
	defer h.lock.Unlock()

	hasContainersRemoved := false

	for _, containerId := range containerIds {
		_, exists := h.managedContainers[containerId]
		if exists {
			delete(h.managedContainers, containerId)
			hasContainersRemoved = true
		}
	}

	if hasContainersRemoved {
		err := h.writer.write(h.managedContainers)
		if err != nil {
			log.Warnf("Got an error while trying to write the config to the prometheus file, err = %v", err)
		}
	}
}

func (h *dockerHandler) findHostPortAndPathForContainer(containerId string) (bool, containerScrapeConfig, error) {

	containerScrapeConfig := containerScrapeConfig{
		Labels: make(map[string]string),
	}

	c, err := h.client.ContainerInspect(context.Background(), containerId)
	if err != nil {
		log.Warnf("Got an error while inspecting container %s: err = %v", containerId, err)
		return false, containerScrapeConfig, err
	}

	if containerIsManaged(c.Config.Labels) {

		// Find port
		port := getFromMapOrDefault(prometheusPortAnnotation, c.Config.Labels, "")
		if port == "" {
			containerPorts := getContainerPortsNetworkConfig(c.NetworkSettings.Ports)
			if len(containerPorts) != 1 {
				log.Warnf("Too many port mapped, can't decide which one to pick without more hint... Please set %s annotation to configure the scraping properly.", prometheusPortAnnotation)
				return false, containerScrapeConfig, nil
			}
			if containerPorts[0].Proto() != "tcp" {
				log.Warnf("The only mapped port is UDP and is not supported by the scraping. Please set %s annotation to configure the scraping properly.", prometheusPortAnnotation)
				return false, containerScrapeConfig, nil
			}
			port = containerPorts[0].Port()
		}

		// Find ip
		var ip string
		network, ok := c.NetworkSettings.Networks[h.config.DockerNetworkName]
		if ok {
			ip = network.IPAddress
		} else {
			ip = getFromMapOrDefault(prometheusIpAnnotation, c.Config.Labels, "")
			if ip == "" {
				if h.config.DockerNetworkStrict {
					log.Warnf("This container is not attached to %s network. Strict networking is requested, so it can't be scraped.", h.config.DockerNetworkName)
					return false, containerScrapeConfig, nil
				}

				log.Infof("This container is not attached to %s network. No guarantee it can be reached by prometheus, but trying the first listed network...", h.config.DockerNetworkName)
				networks := getNetworkNamesFromNetworks(c.NetworkSettings.Networks)
				if len(networks) <= 0 {
					log.Warnf("[BOGUS] No network found at all for this container ?? This container can't be scraped...")
					return false, containerScrapeConfig, nil
				}
				network, ok = c.NetworkSettings.Networks[networks[0]]
				if !ok {
					log.Warnf("[BOGUS] Network %s not found for this container ?? This container can't be scraped...", networks[0])
					return false, containerScrapeConfig, nil
				}
				log.Warnf("Network %s got chosen instead of %s... Hope it is reachable from Prometheus", networks[0], h.config.DockerNetworkName)
				ip = network.IPAddress
			}
		}
		containerScrapeConfig.Targets = append(containerScrapeConfig.Targets, fmt.Sprintf("%s:%s", ip, port))

		// Handle labels
		// Add common labels. Note they can be overwritten by specific labels from the container itself.
		for k, v := range h.config.PrometheusCommonLabels {
			containerScrapeConfig.Labels[k] = v
		}

		extraLabels := parseCSLabels(getFromMapOrDefault(prometheusExtraLabelsAnnotation, c.Config.Labels, ""))
		for k, v := range extraLabels {
			containerScrapeConfig.Labels[k] = v
		}

		if h.config.PrometheusAddContainerMetadata {
			containerScrapeConfig.Labels[model.MetaLabelPrefix+"container_id"] = c.ID
			containerScrapeConfig.Labels[model.MetaLabelPrefix+"container_name"] = c.Name
		}

		// Find path
		path := getFromMapOrDefault(prometheusPathAnnotation, c.Config.Labels, "")
		if path != "" {
			containerScrapeConfig.Labels[model.MetricsPathLabel] = path
		}

		// Find scheme
		scheme := getFromMapOrDefault(prometheusSchemeAnnotation, c.Config.Labels, "")
		if scheme != "" {
			containerScrapeConfig.Labels[model.SchemeLabel] = scheme
		}

		return true, containerScrapeConfig, nil
	} else {
		// container is not managed.
		return false, containerScrapeConfig, nil
	}
}

func containerIsManaged(labels map[string]string) bool {
	enabled, ok := labels[prometheusEnableScrapeAnnotation]
	return ok &&
		enabled == prometheusEnableScrapeAnnotationValue
}
