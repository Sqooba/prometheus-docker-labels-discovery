package main

import (
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"strings"
)

func getFromMapOrDefault(key string, m map[string]string, defaultValue string) string {
	v, ok := m[key]
	if ok {
		return v
	} else {
		return defaultValue
	}
}

func getContainerPortsNetworkConfig(ports nat.PortMap) []nat.Port {
	keys := make([]nat.Port, len(ports))
	i := 0
	for p := range ports {
		keys[i] = p
		i++
	}
	return keys
}

func getNetworkNamesFromNetworks(m map[string]*network.EndpointSettings) []string {
	keys := make([]string, len(m))
	i := 0
	for k := range m {
		keys[i] = k
		i++
	}
	return keys
}

// parseCSLabels parses a string of the format k1:v1,k2:k2,...
func parseCSLabels(labels string) map[string]string {
	m := make(map[string]string)
	if labels == "" {
		return m
	}
	ss := strings.Split(labels, ",")
	for _, pair := range ss {
		z := strings.Split(pair, ":")
		if len(z) < 2 {
			m[z[0]] = ""
		} else {
			m[z[0]] = strings.Join(z[1:], ":")
		}
	}
	return m
}
