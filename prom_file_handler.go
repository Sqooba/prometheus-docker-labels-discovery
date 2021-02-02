package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"syscall"
)

type promHandler struct {
	config envConfig
}

func newPromFileHandler(config envConfig) (*promHandler, error) {

	if !strings.HasSuffix(config.PrometheusConfigFilePath, ".json") {
		return nil, fmt.Errorf("file dose not have .json extension")
	}

	err := syscall.Access(config.PrometheusConfigFilePath, syscall.O_RDWR)
	if err != nil {
		return nil, err
	}

	handler := &promHandler{
		config: config,
	}

	return handler, nil
}

func (h *promHandler) write(containersScrapeConfig map[string]containerScrapeConfig) error {

	targetGroups := make([]containerScrapeConfig, 0, len(containersScrapeConfig))
	for _, v := range containersScrapeConfig {
		targetGroups = append(targetGroups, v)
	}
	data, err := json.Marshal(targetGroups)
	if err != nil {
		return err
	}
	log.Debugf("Write %s to %s", string(data), h.config.PrometheusConfigFilePath)
	err = ioutil.WriteFile(h.config.PrometheusConfigFilePath, data, 0644)
	if err != nil {
		return err
	}

	return err
}
