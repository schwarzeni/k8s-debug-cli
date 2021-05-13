package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

var containerConnChan = make(chan types.HijackedResponse)
var debugFinishChan = make(chan struct{})

func main() {
	go tcpServer()

	// get container id from env
	targetContainerID, ok := os.LookupEnv("ENV_CONTAINER_ID")
	if !ok {
		log.Fatal("unable to find target container name in env")
	}

	// get docker client
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.WithHost("unix:///var/run/docker.sock"), client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatal("new docker client ", err)
	}

	// search for target container by id
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

	// download busybox image
	images, err := cli.ImageList(ctx, types.ImageListOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key:   "reference",
			Value: "busybox:latest",
		}),
	})
	if err != nil {
		log.Fatal("search image ", err)
	}
	if len(images) == 0 {
		if _, err := cli.ImagePull(ctx, "docker.io/library/busybox:latest", types.ImagePullOptions{}); err != nil {
			log.Println("pull image ", err)
		}
		log.Println("pull image success")
	} else {
		log.Println("find local image cache")
	}

	// create busybox container and join to target container ns
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

	// start container
	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		log.Fatal("start container ", err)
	}
	log.Println("start container success")

	// start exec -it
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

	containerConnChan <- attchResp

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		_ = cli.ContainerRemove(ctx, resp.ID, types.ContainerRemoveOptions{Force: true})
		os.Exit(0)
	}()
	<-debugFinishChan
	_ = cli.ContainerRemove(ctx, resp.ID, types.ContainerRemoveOptions{Force: true})
}

func tcpServer() {
	defer func() { debugFinishChan <- struct{}{} }()
	listen, err := net.Listen("tcp", "0.0.0.0:30000")
	if err != nil {
		log.Println("listen failed, err:", err)
		return
	}
	defer listen.Close()
	conn, err := listen.Accept()
	if err != nil {
		log.Println("accept failed, err:", err)
	}
	defer conn.Close()
	cconn := <-containerConnChan

	go func() {
		_, _ = io.Copy(cconn.Conn, conn)
	}()
	_, _ = stdcopy.StdCopy(conn, conn, cconn.Conn)
	// TODO: 一直未退出 。。不知道为什么
	log.Println("shutting down tcp server")
}


// containerd's version
//func main() {
//	targetContainerName, ok := os.LookupEnv("ENV_CONTAINER_NAME")
//	if !ok {
//		log.Fatal("unable to find target container name in env")
//	}
//	containerdNS, ok := os.LookupEnv("ENV_CONTAINERD_NS")
//	if !ok {
//		log.Fatal("unable to find containerd's NS in env")
//	}
//	ctx := namespaces.WithNamespace(context.TODO(), containerdNS)
//
//	client, err := containerd.New("/run/containerd/containerd.sock")
//	//client, err := containerd.New("/run_containerd/containerd.sock")
//	if err != nil {
//		log.Fatal("connect with containerd ", err)
//	}
//	defer client.Close()
//
//	_ = client
//	log.Println("connect success")
//
//	findContainers, err := client.Containers(ctx)
//	//findContainers, err := client.Containers(ctx, "name=="+targetContainerName)
//	if err != nil {
//		log.Fatal("find container ", err)
//	}
//	if len(findContainers) == 0 {
//		log.Fatal("unable to find container with name ", findContainers)
//	}
//	log.Println("======= debug")
//	for i := range findContainers {
//		task, _ := findContainers[i].Task(ctx, nil)
//		fmt.Printf("%+v\n", task)
//		fmt.Printf("%+v\n", findContainers[i])
//		fmt.Println()
//	}
//	_ = targetContainerName
//	return
//
//	targetContainer := findContainers[0]
//	task, err := targetContainer.Task(ctx, nil)
//	if err != nil {
//		log.Fatal("get current container task ", err)
//	}
//	targetContainerPID := task.Pid()
//	log.Println("fetch container pid ", targetContainerPID)
//
//}
