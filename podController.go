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

// PodController represents a controller used for approving csrs
type PodController struct {
    kubeClient clientset.Interface

    podLister  listers.PodLister
    podsSynced cache.InformerSynced

    handler func(*corev1.Pod) error

    queue workqueue.RateLimitingInterface
}

// NewPodController creates a new csr approving controller
func NewPodController(
    client clientset.Interface,
    informer informers.PodInformer,
    handler func(*corev1.Pod) error,
) *PodController {

    cc := &PodController{
        kubeClient: client,
        queue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "pods"),
        handler:    handler,
    }

    // Manage the addition/update of certificate requests
    informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
        AddFunc: func(obj interface{}) {
            pod := obj.(*corev1.Pod)
            logrus.Debugf("Adding pod %s", pod.Name)
            cc.enqueue(obj)
        },
        UpdateFunc: func(old, new interface{}) {
            oldPod := old.(*corev1.Pod)
            logrus.Debugf("Updating pod %s", oldPod.Name)
            cc.enqueue(new)
        },
        DeleteFunc: func(obj interface{}) {
            pod, ok := obj.(*corev1.Pod)
            if !ok {
                tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
                if !ok {
                    logrus.Debugf("Couldn't get object from tombstone %#v", obj)
                    return
                }
                pod, ok = tombstone.Obj.(*corev1.Pod)
                if !ok {
                    logrus.Debugf("Tombstone contained object that is not a pod: %#v", obj)
                    return
                }
            }
            logrus.Debugf("Deleting pod %s", pod.Name)
            cc.enqueue(obj)
        },
    })
    cc.podLister = informer.Lister()
    cc.podsSynced = informer.Informer().HasSynced
    return cc
}

// Run the controller workers.
func (cc *PodController) Run(workers int, stopCh <-chan struct{}) {
    defer utilruntime.HandleCrash()
    defer cc.queue.ShutDown()

    logrus.Infof("Starting pod controller")
    defer logrus.Infof("Shutting down pod controller")

    if !cache.WaitForCacheSync(stopCh, cc.podsSynced) {
        return
    }

    for i := 0; i < workers; i++ {
        go wait.Until(cc.runWorker, time.Second, stopCh)
    }

    <-stopCh
}

func (cc *PodController) runWorker() {
    for cc.processNextWorkItem() {
    }
}

func (cc *PodController) processNextWorkItem() bool {
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

func (cc *PodController) enqueue(obj interface{}) {
    key, err := cache.MetaNamespaceKeyFunc(obj)
    if err != nil {
        utilruntime.HandleError(fmt.Errorf("Couldn't get key for object %+v: %v", obj, err))
        return
    }
    cc.queue.Add(key)
}

func (cc *PodController) sync(key string) error {
    startTime := time.Now()

    namespace, name, err := cache.SplitMetaNamespaceKey(key)
    if err != nil {
        utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
        return nil
    }

    defer func() {
        logrus.Debugf("Finished syncing pod %q (%v)", key, time.Since(startTime))
    }()

    pod, err := cc.podLister.Pods(namespace).Get(name)
    if errors.IsNotFound(err) {
        logrus.Debugf("pod has been deleted: %v", key)
        return nil
    }
    if err != nil {
        return err
    }

    // need to operate on a copy so we don't mutate the pvc in the shared cache
    pod = pod.DeepCopy()

    err = cc.handler(pod)
    if err != nil {
        return fmt.Errorf("couldn't sync pod `%s` in namespace `%s`, see: %v", name, namespace, err)
    }

    return nil
}
