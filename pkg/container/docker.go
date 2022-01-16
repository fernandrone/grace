package container

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/client"
	docker "github.com/docker/docker/client"
)

// DockerContainer represents a Docker container.
type DockerContainer struct {
	*docker.Client
	ContainerID string
}

const dockerDefaultStopTimeout = time.Second * time.Duration(10)

func NewDockerContainer(containerID string) (Containers, error) {
	c, err := docker.NewClientWithOpts(client.FromEnv)

	if err != nil {
		return nil, err
	}

	container := &DockerContainer{
		Client:      c,
		ContainerID: containerID,
	}

	return container, nil
}

func (c DockerContainer) Stop(ctx context.Context) ([]Response, error) {
	json, err := c.ContainerInspect(ctx, c.ContainerID)

	if err != nil {
		return []Response{}, err
	}

	shortID := json.ID[:15]

	// see if is runnning
	if !json.State.Running {
		return []Response{}, fmt.Errorf("container %s is not running", shortID)
	}

	start := time.Now()

	// try to gracefully stop the container
	err = c.ContainerStop(ctx, c.ContainerID, nil)

	duration := time.Since(start)

	if err != nil {
		return []Response{}, err
	}

	json, err = c.ContainerInspect(ctx, c.ContainerID)

	if err != nil {
		return []Response{}, err
	}

	terms := json.Config.Entrypoint
	terms = append(terms, json.Config.Cmd...)
	command := strings.Join(terms, " ")

	if len(command) > 30 {
		command = command[0:27] + "..."
	}

	stopTimeout := dockerDefaultStopTimeout
	if json.Config.StopTimeout != nil {
		stopTimeout = time.Second * time.Duration(*json.Config.StopTimeout)
	}

	r := []Response{{
		Config: Config{
			ID:          shortID,
			Image:       json.Config.Image,
			Command:     command,
			StopTimeout: stopTimeout,
		},
		ExitCode:  json.State.ExitCode,
		Duration:  duration,
		OOMKilled: json.State.OOMKilled,
	}}

	return r, nil
}
