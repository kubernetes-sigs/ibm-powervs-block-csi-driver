## E2E Testing
E2E test verifies the funcitonality of IBM PowerVS Block CSI Driver in the context of Kubernetes. It exercises driver feature e2e including static provisioning, dynamic provisioning, volume scheduling, mount options, etc.

### Notes
Tests marked with `[env]` require specific environmental variables to be set, if not set these tests will be skipped.

```
export IBMCLOUD_API_KEY=XXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
```

Tests marked with `[labels]` requires specific labels to be set for each node of the cluster, if not set these tests will be skipped.

```
kubectl label nodes <node-name> powervs.kubernetes.io/cloud-instance-id=<GUID of the workspace>
kubectl label nodes <node-name> powervs.kubernetes.io/pvm-instance-id=<Instance ID of the PowerVS instance>
```

Set the ProviderID on the cluster nodes as: `ibmpowervs://<region>/<zone>/<service_instance_id>/<powervs_machine_id>`, for example:
```sh
spec:
  providerID: ibmpowervs://syd/syd05/862032d5-xxxx-xxxx-xxxx-c18594456427/2c6cbaec-xxxx-xxxx-xxxx-a6aa35315596
```

- The **[Node Controller](https://kubernetes.io/docs/concepts/architecture/cloud-controller/#node-controller)** of the the **[CloudControllerManager](https://kubernetes.io/docs/concepts/architecture/cloud-controller)** is responsible for labelling the nodes.