package task

import (
	"context"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"
	"io"
	"log"
	"os"
	"time"
)

type State int

const (
	Pending State = iota
	Scheduled
	Completed
	Running
	Failed
)

type Task struct {
	ID            uuid.UUID
	ContainerID   string
	Name          string
	State         State
	Image         string
	Cpu           float64
	Memory        int64
	Disk          int64
	ExposedPorts  nat.PortSet
	PortBindings  map[string]string
	RestartPolicy string
	StartTime     time.Time
	FinishTime    time.Time
}

type TaskEvent struct {
	ID        uuid.UUID
	State     State
	Timestamp time.Time
	Task      Task
}

type Config struct {
	Name          string
	AttachStdin   bool
	AttachStdout  bool
	AttachStderr  bool
	ExposedPorts  nat.PortSet
	Cmd           []string
	Image         string
	Cpu           float64
	Memory        int64
	Disk          int64
	RestartPolicy string
	Env           []string
}

func NewConfig(t *Task) *Config {
	return &Config{
		Name:          t.Name,
		ExposedPorts:  t.ExposedPorts,
		Image:         t.Image,
		Cpu:           t.Cpu,
		Memory:        t.Memory,
		Disk:          t.Disk,
		RestartPolicy: t.RestartPolicy,
	}
}

type Docker struct {
	Client *client.Client
	Config Config
}

func NewDocker(c *Config) *Docker {
	dc, _ := client.NewClientWithOpts(client.FromEnv)
	return &Docker{
		Client: dc,
		Config: *c,
	}
}

type DockerResult struct {
	Error       error
	Action      string
	ContainerId string
	Result      string
}

func (d *Docker) Run() DockerResult {
	ctx := context.Background()
	reader, err := d.Client.ImagePull(ctx, d.Config.Image, types.ImagePullOptions{})
	if err != nil {
		log.Printf("Error pulling image %s: %v\n", d.Config.Image, err)
		return DockerResult{Error: err}
	}
	io.Copy(os.Stdout, reader)

	rp := container.RestartPolicy{
		Name: d.Config.RestartPolicy,
	}

	r := container.Resources{
		Memory: d.Config.Memory,
	}

	cc := container.Config{
		Image: d.Config.Image,
		Env:   d.Config.Env,
	}

	hc := container.HostConfig{
		RestartPolicy:   rp,
		Resources:       r,
		PublishAllPorts: true,
	}

	resp, err := d.Client.ContainerCreate(ctx, &cc, &hc, nil, nil, d.Config.Name)
	if err != nil {
		log.Printf("Error creating container user image %s: %v\n", d.Config.Image, err)
		return DockerResult{Error: err}
	}

	err = d.Client.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})
	if err != nil {
		log.Printf("Error starting container %s: %v\n", resp.ID, err)
		return DockerResult{Error: err}
	}

	out, err := d.Client.ContainerLogs(
		ctx,
		resp.ID,
		types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true},
	)
	if err != nil {
		log.Printf("Error getting logs for container %s: %v\n", resp.ID, err)
		return DockerResult{Error: err}
	}

	stdcopy.StdCopy(os.Stdout, os.Stderr, out)

	return DockerResult{
		ContainerId: resp.ID,
		Action:      "start",
		Result:      "success",
	}
}

func (d *Docker) Stop(containerID string) DockerResult {
	ctx := context.Background()
	log.Printf("Attempting to stop container %v", containerID)
	err := d.Client.ContainerStop(ctx, containerID, nil)
	if err != nil {
		panic(err)
	}

	removeOptions := types.ContainerRemoveOptions{
		RemoveVolumes: true,
		RemoveLinks:   false,
		Force:         false,
	}

	err = d.Client.ContainerRemove(ctx, containerID, removeOptions)
	if err != nil {
		panic(err)
	}

	return DockerResult{Action: "stop", Result: "success", Error: nil}
}
