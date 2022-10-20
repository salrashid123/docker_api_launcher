package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

var (
	username = flag.String("username", "", "registry username")
	token    = flag.String("token", "", "registry auth token")
	image    = flag.String("image", "", "registry image")
)

func main() {
	flag.Parse()

	if *username == "" || *token == "" || *image == "" {
		fmt.Printf("username, token and image must be specified")
		os.Exit(1)
	}
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		fmt.Printf("Error creating docker client %v\n", err)
		os.Exit(1)
	}

	authConfig := types.AuthConfig{
		Username:      *username,
		RegistryToken: *token,
	}
	encodedJSON, err := json.Marshal(authConfig)
	if err != nil {
		fmt.Printf("Error marshalling docker creds %v\n", err)
		os.Exit(1)
	}
	authStr := base64.URLEncoding.EncodeToString(encodedJSON)

	reader, err := cli.ImagePull(ctx, *image, types.ImagePullOptions{
		RegistryAuth: authStr,
	})
	if err != nil {
		fmt.Printf("Error pulling image %v\n", err)
		os.Exit(1)
	}

	defer reader.Close()
	io.Copy(os.Stdout, reader)

	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			"8080/tcp": []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: "8080",
				},
			},
		},
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: *image,
		ExposedPorts: nat.PortSet{
			"8080/tcp": struct{}{},
		},
	}, hostConfig, nil, nil, "")
	if err != nil {
		fmt.Printf("Error creating container %v\n", err)
		os.Exit(1)
	}

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		fmt.Printf("Error starting container %v\n", err)
		os.Exit(1)
	}

	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			fmt.Printf("Failed error container wait %v\n", err)
			os.Exit(1)
		}
	case <-statusCh:
	}

	out, err := cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStdout: true})
	if err != nil {
		fmt.Printf("Error on container logs docker client %v\n", err)
		os.Exit(1)
	}

	stdcopy.StdCopy(os.Stdout, os.Stderr, out)
}
