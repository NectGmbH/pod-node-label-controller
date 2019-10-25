package main

import (
    "flag"
    "strings"
    "sync"
    "time"

    "github.com/sirupsen/logrus"

    "k8s.io/client-go/informers"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/clientcmd"
)

type sliceFlags []string

func (i *sliceFlags) String() string {
    return strings.Join(*i, " ")
}

func (i *sliceFlags) Set(value string) error {
    *i = append(*i, value)
    return nil
}

func main() {
    var kubeconfig string
    var masterURL string
    var labels sliceFlags

    flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
    flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
    flag.Var(&labels, "label", "Label to sync from node to pod.")
    flag.Parse()

    cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
    if err != nil {
        logrus.Fatalf("Error building kubeconfig: %s", err.Error())
    }

    kubeClient, err := kubernetes.NewForConfig(cfg)
    if err != nil {
        logrus.Fatalf("Error building kubernetes clientset: %s", err.Error())
    }

    kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, time.Second*30)

    labeler := NewLabeler(kubeClient, labels)

    podController := NewPodController(
        kubeClient,
        kubeInformerFactory.Core().V1().Pods(),
        labeler.HandlePod,
    )

    nodeController := NewNodeController(
        kubeClient,
        kubeInformerFactory.Core().V1().Nodes(),
        labeler.HandleNode,
    )

    stopCh := make(chan struct{})
    defer close(stopCh)

    kubeInformerFactory.Start(stopCh)

    wg := &sync.WaitGroup{}
    wg.Add(2)

    go (func() {
        podController.Run(2, stopCh)
        logrus.Fatalf("pod controller stopped")
        wg.Done()
    })()

    go (func() {
        nodeController.Run(2, stopCh)
        logrus.Fatalf("node controller stopped")
        wg.Done()
    })()

    wg.Wait()
}
