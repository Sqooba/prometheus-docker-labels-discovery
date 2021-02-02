package main

import (
	"github.com/sqooba/go-common/logging"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDockerEventsListener(t *testing.T) {

	config := envConfig{
		PrometheusConfigFilePath: "test-data/from-docker-labels.json",
		DockerNetworkName:        "bridge",
		LogLevel:                 "debug",
		PrometheusCommonLabels:   map[string]string{"c1": "v1"},
	}
	err := logging.SetLogLevel(log, config.LogLevel)

	writer, err := newPromFileHandler(config)
	assert.Nil(t, err)

	dockerHandler, err := newDockerHandler(config, writer)
	assert.Nil(t, err)

	err = dockerHandler.handle()
	assert.Nil(t, err)
}
