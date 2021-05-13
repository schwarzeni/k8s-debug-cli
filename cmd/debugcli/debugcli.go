package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh/terminal"
	"k8s.io/client-go/tools/clientcmd"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
)

var (
	kubeconfig      = flag.String("f", "/Users/nizhenyang/Desktop/k8s-pod-debug/kubeconfig.yaml", "absolute path to the kubeconfig file")
	podName         = flag.String("p", "kube-scheduler-k8s-master", "pod's name")
	podNS           = flag.String("n", "kube-system", "pod's namespace")
	debugAgentPodNS = corev1.NamespaceDefault // TODO: just in default ns
)

const (
	DEBUG_AGENT_IMAGE = "10.211.55.2:10000/debug-agent:v0.0.1"
	DEBUG_AGENT_PORT  = 30000
)

func init() {
	flag.Parse()
}

func main() {
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		log.Fatal("init config", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal("init client", err)
	}

	targetPod, err := clientset.CoreV1().Pods(*podNS).Get(context.TODO(), *podName, metav1.GetOptions{})
	if err != nil {
		log.Fatal("get target pod", *podName, "in namespace", *podNS, err)
	}
	if targetPod == nil {
		log.Fatal("target pod " + *podName + " not exist in ns " + *podNS)
	}

	daPod, err := clientset.CoreV1().Pods(debugAgentPodNS).Create(context.TODO(), newDebugAgentPod(targetPod), metav1.CreateOptions{})
	if err != nil {
		log.Fatal("create debug agent pod", err)
	}
	log.Println("create debug agent pod", daPod.Name, "success in ns", daPod.Namespace)

	daPod, err = waitForPodStart(context.TODO(), clientset, daPod)
	if err != nil {
		log.Fatal("wait for debug agent pod start", err)
	}
	log.Println("debug agent pod is start, try to get conn ..")

	// get ip
	addr := fmt.Sprintf("%s:%d", daPod.Status.HostIP, DEBUG_AGENT_PORT)
	var conn net.Conn
	retry := 5
	for {
		conn, err = net.Dial("tcp", addr)
		if err == nil {
			break
		}
		if retry--; retry == 0 {
			log.Fatal("dial to debug agent at ", addr, "failed", err)
		}
		time.Sleep(time.Second)
	}
	defer conn.Close()

	if !terminal.IsTerminal(0) || !terminal.IsTerminal(1) {
		fmt.Errorf("stdin/stdout should be terminal")
	}
	oldState, err := terminal.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer terminal.Restore(0, oldState)

	go func() {
		if _, err := io.Copy(conn, os.Stdin); err != nil {
			log.Println("io.Copy(conn, os.Stdin)", err)
		}
	}()
	if _, err := io.Copy(os.Stdout, conn); err != nil {
		log.Println("io.Copy(conn, os.Stdin)", err)
	}
	log.Println("client exit")

	_ = clientset.CoreV1().Pods(debugAgentPodNS).Delete(context.TODO(), daPod.Name, metav1.DeleteOptions{})
}
func newDebugAgentPod(targetPod *corev1.Pod) *corev1.Pod {
	var runAsUserNum int64 = 0

	getContainerID := func() string {
		// TODO: target at first container in the pod ）
		targetContainerName := targetPod.Spec.Containers[0].Name
		for _, s := range targetPod.Status.ContainerStatuses {
			if s.Name == targetContainerName {
				// docker://.....
				return s.ContainerID[9:]
			}
		}
		log.Fatal("get container id not found ", targetContainerName)
		return ""
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("debug-agent-for-%s", targetPod.Name),
			Namespace:    debugAgentPodNS,
			Labels:       map[string]string{"debug-agent": targetPod.Name},
		},
		Spec: corev1.PodSpec{
			HostNetwork: true,
			NodeName:    targetPod.Spec.NodeName,
			Volumes: []corev1.Volume{
				{
					Name: "cri-sock",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{Path: "/var/run/docker.sock"},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "debug-agent-container",
					Image: DEBUG_AGENT_IMAGE,
					SecurityContext: &corev1.SecurityContext{
						RunAsUser: &runAsUserNum,
					},
					Ports: []corev1.ContainerPort{
						{ContainerPort: DEBUG_AGENT_PORT, HostPort: DEBUG_AGENT_PORT},
					},
					Env: []corev1.EnvVar{
						{
							Name:  "ENV_CONTAINER_ID",
							Value: getContainerID(),
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "cri-sock",
							MountPath: "/var/run/docker.sock",
						},
					},
				},
			},
		},
	}
}

// waitForPodStart
// ref: https://github.com/feiskyer/go-examples/blob/master/kubernetes/pod-create/pod.go#L27
func waitForPodStart(ctx context.Context, client *kubernetes.Clientset, pod *corev1.Pod) (*corev1.Pod, error) {
	w, err := client.CoreV1().Pods(pod.Namespace).Watch(ctx, metav1.ListOptions{
		Watch:           true,
		ResourceVersion: pod.ResourceVersion,
		FieldSelector:   fields.Set{"metadata.name": pod.Name}.String(),
	})
	if err != nil {
		return nil, err
	}

	status := pod.Status
	func() {
		for {
			select {
			case events, ok := <-w.ResultChan():
				if !ok {
					return
				}
				pod = events.Object.(*corev1.Pod)
				status = pod.Status
				if pod.Status.Phase != corev1.PodPending {
					w.Stop()
				}
			case <-time.After(20 * time.Second):
				log.Println("timeout to wait for pod active")
				w.Stop()
			}
		}
	}()
	if status.Phase != corev1.PodRunning {
		return nil, fmt.Errorf("Pod is unavailable: %v", status.Phase)
	}
	pod.Status = status
	return pod, nil
}
