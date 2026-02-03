package dockerx

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

type Client struct {
	enabled bool
	c       *client.Client
}

func New() (*Client, error) {
	if os.Getenv("FORGE_DROP_NO_DOCKER") == "1" {
		return &Client{enabled: false}, nil
	}
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return &Client{enabled: false}, err
	}
	return &Client{enabled: true, c: c}, nil
}

func (c *Client) Enabled() bool { return c != nil && c.enabled && c.c != nil }

func (c *Client) Close() error {
	if c == nil || c.c == nil {
		return nil
	}
	return c.c.Close()
}

func (c *Client) Ping(ctx context.Context) error {
	if !c.Enabled() {
		return errors.New("docker disabled")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := c.c.Ping(ctx)
	return err
}

func (c *Client) ListContainers(ctx context.Context, all bool, labelEquals map[string]string) ([]types.Container, error) {
	if !c.Enabled() {
		return nil, errors.New("docker disabled")
	}
	f := filters.NewArgs()
	for k, v := range labelEquals {
		f.Add("label", k+"="+v)
	}
	return c.c.ContainerList(ctx, container.ListOptions{All: all, Filters: f})
}

func (c *Client) InspectContainer(ctx context.Context, id string) (types.ContainerJSON, error) {
	if !c.Enabled() {
		return types.ContainerJSON{}, errors.New("docker disabled")
	}
	return c.c.ContainerInspect(ctx, id)
}

func (c *Client) RemoveContainer(ctx context.Context, id string, force bool) error {
	if !c.Enabled() {
		return errors.New("docker disabled")
	}
	return c.c.ContainerRemove(ctx, id, container.RemoveOptions{Force: force, RemoveVolumes: true})
}

func (c *Client) RestartContainer(ctx context.Context, id string) error {
	if !c.Enabled() {
		return errors.New("docker disabled")
	}
	timeout := 10
	return c.c.ContainerRestart(ctx, id, container.StopOptions{Timeout: &timeout})
}

func (c *Client) StopContainer(ctx context.Context, id string) error {
	if !c.Enabled() {
		return errors.New("docker disabled")
	}
	timeout := 10
	return c.c.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout})
}

func (c *Client) CreateContainer(ctx context.Context, cfg container.Config, hostCfg container.HostConfig, netCfg network.NetworkingConfig, name string) (container.CreateResponse, error) {
	if !c.Enabled() {
		return container.CreateResponse{}, errors.New("docker disabled")
	}
	return c.c.ContainerCreate(ctx, &cfg, &hostCfg, &netCfg, nil, name)
}

func (c *Client) StartContainer(ctx context.Context, id string) error {
	if !c.Enabled() {
		return errors.New("docker disabled")
	}
	return c.c.ContainerStart(ctx, id, container.StartOptions{})
}
