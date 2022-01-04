## E2E Testing
E2E test verifies the funcitonality of IBM PowerVS Block CSI Driver in the context of Kubernetes. It exercises driver feature e2e including static provisioning, dynamic provisioning, volume scheduling, mount options, etc.

### Notes
Some tests marked with `[env]` require specific environmental variables to be set, if not set these tests will be skipped.

```
export IBMCLOUD_API_KEY=XXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
```

Some tests marked with `[labels]` require specific labels to be set for each node of the cluster, if not set these tests will be skipped.
```
kubectl label nodes multinode-kube-master powervs.kubernetes.io/cloud-instance-id=7845d372-d4e1-46b8-91fc-41051c984601
kubectl label nodes multinode-kube-master powervs.kubernetes.io/pvm-instance-id=638667f8-a4d3-46d0-9fa6-ddc621100407
```

- The **[Node Controller](https://kubernetes.io/docs/concepts/architecture/cloud-controller/#node-controller)** of the the **[CloudControllerManager](https://kubernetes.io/docs/concepts/architecture/cloud-controller)** is responsible for labelling the nodes. The above one is used as temporary fix.