package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"golang.org/x/term"
)

func main() {
	// find container id
	targetContainerID, ok := os.LookupEnv("ENV_CONTAINER_ID")
	if !ok {
		log.Fatal("unable to find target container name in env")
	}

	// connect to docker
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.WithHost("unix:///var/run/docker.sock"), client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatal("new docker client ", err)
	}
	log.Println("connect to docker")

	// find target container by id
	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key:   "id",
			Value: targetContainerID,
		}),
	})
	if err != nil {
		log.Fatal("list containers ", err)
	}
	if len(containers) == 0 {
		log.Fatal("unable to find container with id " + targetContainerID)
	}
	targetContainer := containers[0]

	// find if image exists
	images, err := cli.ImageList(ctx, types.ImageListOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key:   "reference",
			Value: "busybox:latest",
		}),
	})
	if err != nil {
		log.Fatal("search image ", err)
	}

	// if not, download it
	if len(images) == 0 {
		if _, err := cli.ImagePull(ctx, "docker.io/library/busybox:latest", types.ImagePullOptions{}); err != nil {
			log.Println("pull image ", err)
		}
		log.Println("pull image success")
	} else {
		log.Println("find local image cache")
	}

	// create container
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: "busybox:latest",
		Cmd:   []string{"bin/sh", "-c", "tail -f /dev/null"},
	}, &container.HostConfig{
		// ref: https://docs.docker.com/engine/reference/run/
		VolumesFrom: []string{targetContainer.ID},
		IpcMode:     container.IpcMode("container:" + targetContainer.ID),
		NetworkMode: container.NetworkMode("container:" + targetContainer.ID),
		PidMode:     container.PidMode("container:" + targetContainer.ID),
		//UTSMode:     "",
		//UsernsMode:  ,
	}, nil, nil, "debug-for_"+targetContainerID+fmt.Sprintf("_%d", time.Now().Unix()))
	if err != nil {
		log.Fatal("create container ", err)
	}
	log.Println("create container success")

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		log.Fatal("start container ", err)
	}
	log.Println("start container success")

	// start exec
	execResp, err := cli.ContainerExecCreate(ctx, resp.ID, types.ExecConfig{
		Tty:          true,
		AttachStdin:  true,
		AttachStderr: true,
		AttachStdout: true,
		Cmd:          []string{"/bin/sh"},
	})
	if err != nil {
		log.Fatal("container exec create ", err)
	}
	log.Println("container exec create success")
	attchResp, err := cli.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{})
	if err != nil {
		log.Fatal("container exec attach ", err)
	}
	defer attchResp.Close()
	log.Println("container exec attach")

	// start cli
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatal("make term stdin raw ", err)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }() // Best effort.

	// get output
	// ref https://stackoverflow.com/questions/52774830/docker-exec-command-from-golang-api
	go func() {
		_, _ = stdcopy.StdCopy(os.Stdout, os.Stderr, attchResp.Conn)
	}()
	_, _ = io.Copy(attchResp.Conn, os.Stdin)

	_ = cli.ContainerRemove(ctx, resp.ID, types.ContainerRemoveOptions{Force: true})
}
