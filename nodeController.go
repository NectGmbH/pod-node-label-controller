package main

import (
    "fmt"
    "time"

    "github.com/sirupsen/logrus"

    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/api/errors"
    utilruntime "k8s.io/apimachinery/pkg/util/runtime"
    "k8s.io/apimachinery/pkg/util/wait"
    informers "k8s.io/client-go/informers/core/v1"
    clientset "k8s.io/client-go/kubernetes"
    listers "k8s.io/client-go/listers/core/v1"
    "k8s.io/client-go/tools/cache"
    "k8s.io/client-go/util/workqueue"
)

// NodeController represents a controller used for approving csrs
type NodeController struct {
    kubeClient clientset.Interface

    nodeLister  listers.NodeLister
    nodesSynced cache.InformerSynced

    handler func(*corev1.Node) error

    queue workqueue.RateLimitingInterface
}

// NewNodeController creates a new csr approving controller
func NewNodeController(
    client clientset.Interface,
    informer informers.NodeInformer,
    handler func(*corev1.Node) error,
) *NodeController {

    cc := &NodeController{
        kubeClient: client,
        queue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "nodes"),
        handler:    handler,
    }

    // Manage the addition/update of certificate requests
    informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
        AddFunc: func(obj interface{}) {
            node := obj.(*corev1.Node)
            logrus.Debugf("Adding node %s", node.Name)
            cc.enqueue(obj)
        },
        UpdateFunc: func(old, new interface{}) {
            oldNode := old.(*corev1.Node)
            logrus.Debugf("Updating node %s", oldNode.Name)
            cc.enqueue(new)
        },
        DeleteFunc: func(obj interface{}) {
            node, ok := obj.(*corev1.Node)
            if !ok {
                tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
                if !ok {
                    logrus.Debugf("Couldn't get object from tombstone %#v", obj)
                    return
                }
                node, ok = tombstone.Obj.(*corev1.Node)
                if !ok {
                    logrus.Debugf("Tombstone contained object that is not a node: %#v", obj)
                    return
                }
            }
            logrus.Debugf("Deleting node %s", node.Name)
            cc.enqueue(obj)
        },
    })
    cc.nodeLister = informer.Lister()
    cc.nodesSynced = informer.Informer().HasSynced
    return cc
}

// Run the controller workers.
func (cc *NodeController) Run(workers int, stopCh <-chan struct{}) {
    defer utilruntime.HandleCrash()
    defer cc.queue.ShutDown()

    logrus.Infof("Starting node controller")
    defer logrus.Infof("Shutting down node controller")

    if !cache.WaitForCacheSync(stopCh, cc.nodesSynced) {
        return
    }

    for i := 0; i < workers; i++ {
        go wait.Until(cc.runWorker, time.Second, stopCh)
    }

    <-stopCh
}

func (cc *NodeController) runWorker() {
    for cc.processNextWorkItem() {
    }
}

func (cc *NodeController) processNextWorkItem() bool {
    cKey, quit := cc.queue.Get()
    if quit {
        return false
    }

    defer cc.queue.Done(cKey)

    if err := cc.sync(cKey.(string)); err != nil {
        cc.queue.AddRateLimited(cKey)
        utilruntime.HandleError(fmt.Errorf("Sync %v failed with : %v", cKey, err))

        return true
    }

    cc.queue.Forget(cKey)
    return true

}

func (cc *NodeController) enqueue(obj interface{}) {
    key, err := cache.MetaNamespaceKeyFunc(obj)
    if err != nil {
        utilruntime.HandleError(fmt.Errorf("Couldn't get key for object %+v: %v", obj, err))
        return
    }
    cc.queue.Add(key)
}

func (cc *NodeController) sync(key string) error {
    startTime := time.Now()

    defer func() {
        logrus.Debugf("Finished syncing node %q (%v)", key, time.Since(startTime))
    }()

    node, err := cc.nodeLister.Get(key)
    if errors.IsNotFound(err) {
        logrus.Debugf("node has been deleted: %v", key)
        return nil
    }
    if err != nil {
        return err
    }

    // need to operate on a copy so we don't mutate the node in the shared cache
    node = node.DeepCopy()

    err = cc.handler(node)
    if err != nil {
        return fmt.Errorf("couldn't sync node `%s`, see: %v", key, err)
    }

    return nil
}
