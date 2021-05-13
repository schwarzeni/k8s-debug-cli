package main

import (
	"context"
	"flag"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	kubeconfigF := flag.String("f", "/Users/nizhenyang/Desktop/k8s-pod-debug/kubeconfig.yaml", "absolute path to the kubeconfig file")
	podNameF := flag.String("p", "kube-scheduler-k8s-master", "pod's name")
	podNSF := flag.String("n", "kube-system", "pod's namespace")
	flag.Parse()
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfigF)
	if err != nil {
		log.Fatal("init config", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal("init client", err)
	}

	targetPod, err := clientset.CoreV1().Pods(*podNSF).Get(context.TODO(), *podNameF, metav1.GetOptions{})
	if err != nil {
		log.Fatal("get target pod", *podNameF, "in namespace", *podNSF, err)
	}
	if targetPod == nil {
		log.Fatal("pod " + *podNameF + " in namespace " + *podNSF + " not found")
	}

	targetPodHostIP := targetPod.Status.HostIP
	targetPodHostName := targetPod.Spec.NodeName
	targetPodContainerID := func() string {
		targetContainerName := targetPod.Spec.Containers[0].Name
		for _, s := range targetPod.Status.ContainerStatuses {
			if s.Name == targetContainerName {
				// docker://.....
				return s.ContainerID[9:]
			}
		}
		log.Fatal("get container id not found ", targetContainerName)
		return ""
	}()
	log.Printf("\ntargetPodHostIP: %s\ntargetPodHostName: %s\ntargetPodContainerID: %s\n",
		targetPodHostIP, targetPodHostName, targetPodContainerID)
}
