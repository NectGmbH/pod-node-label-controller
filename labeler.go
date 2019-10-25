package main

import (
    "fmt"
    "sync"

    "github.com/sirupsen/logrus"

    corev1 "k8s.io/api/core/v1"
    clientset "k8s.io/client-go/kubernetes"
)

// Labeler is the business logic which actually takes care of labeling the pods
type Labeler struct {
    sync.RWMutex
    pods   map[string]*corev1.Pod
    nodes  map[string]*corev1.Node
    labels []string
    client clientset.Interface
}

// NewLabeler creates a new labeler instance which syncs node labels to pod labels
func NewLabeler(client clientset.Interface, labels []string) *Labeler {
    return &Labeler{
        client: client,
        labels: labels,
        pods:   make(map[string]*corev1.Pod),
        nodes:  make(map[string]*corev1.Node),
    }
}

// HandlePod syncs the passed pod with the labelers storage
func (l *Labeler) HandlePod(pod *corev1.Pod) error {
    l.Lock()
    defer l.Unlock()

    key := pod.GetNamespace() + "/" + pod.GetName()

    if pod.DeletionTimestamp != nil {
        delete(l.pods, key)
    } else {
        l.pods[key] = pod
    }

    return l.labelPod(pod)
}

// HandleNode syncs the passed node with the labelers storage
func (l *Labeler) HandleNode(node *corev1.Node) error {
    l.Lock()
    defer l.Unlock()

    if node.DeletionTimestamp != nil {
        delete(l.nodes, node.GetName())
    } else {
        l.nodes[node.GetName()] = node
    }

    var lastErr error

    for _, pod := range l.pods {
        key := pod.GetNamespace() + "/" + pod.GetName()
        err := l.labelPod(pod)
        if err != nil {
            logrus.Warnf("couldn't update pod `%s` after change of node `%s`, see: %v", key, node.GetName(), err)
            lastErr = err
        }
    }

    return lastErr
}

func (l *Labeler) labelPod(pod *corev1.Pod) error {
    key := pod.GetNamespace() + "/" + pod.GetName()

    nodeName := pod.Spec.NodeName
    if nodeName == "" {
        return fmt.Errorf("pod `%s` isn't scheduled to a node yet, therefore can't label it", key)
    }

    node, ok := l.nodes[nodeName]
    if !ok {
        return fmt.Errorf("pod `%s` couldn't be labeled since the node is not known (yet)", key)
    }

    dirty := false

    podLabels := pod.GetLabels()
    nodeLables := node.GetLabels()
    for _, labelName := range l.labels {
        labelValue, ok := nodeLables[labelName]
        if !ok {
            logrus.Debugf("label `%s` missing on node `%s`", labelName, nodeName)
            continue
        }

        if val, ok := podLabels[labelName]; !ok || val != labelValue {
            dirty = true
        }

        podLabels[labelName] = labelValue
    }

    if !dirty {
        logrus.Debugf("skipping update of pod `%s` since nothing changed", key)
        return nil
    }

    pod = pod.DeepCopy()
    pod.SetLabels(podLabels)

    pod, err := l.client.CoreV1().Pods(pod.GetNamespace()).Update(pod)
    if err != nil {
        return err
    }

    logrus.Infof("Synced node labels from node `%s` to pod `%s`", nodeName, key)

    l.pods[key] = pod

    return nil
}
