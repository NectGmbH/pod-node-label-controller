# pod-node-label-controller
Kubernetes controller for automatical synchronization from node labels to pods.
Usage scenario would be e.g. adding information of the availability zone to the individual pods.

## Deploy using helm
```
$ helm upgrade -i pod-node-label-controller --namespace pod-node-label-controller ./charts/pod-node-label-controller -f my-values.yaml
```

### Values

| Key      | Default value                                                                         | Description               |
| ---------| ------------------------------------------------------------------------------------- | ------------------------- |
| image    | 'kavatech/pod-node-label-controller:v0.1.0'                                           | Image of the container    |
| labels   | ['failure-domain.beta.kubernetes.io/region','failure-domain.beta.kubernetes.io/zone'] | Labels to synchronize     |
