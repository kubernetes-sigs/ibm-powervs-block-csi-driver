# Static Provisioning 
This example shows how to create and consume persistent volume from exising PowerVS using static provisioning.

## Usage
1. Edit the PersistentVolume spec in [example manifest](./specs/example.yaml). Update `volumeHandle` with PowerVS volume ID that you are going to use, and update the `fsType` with the filesystem type of the volume. In this example, I have a pre-created PowerVS volume and it is formatted with xfs filesystem.

```
apiVersion: v1
kind: PersistentVolume
metadata:
  name: test-pv
spec:
  capacity:
    storage: 50Gi
  volumeMode: Filesystem
  accessModes:
    - ReadWriteOnce
  csi:
    driver: powervs.csi.ibm.com
    volumeHandle: {volumeId} 
    fsType: xfs
```

2. Deploy the example:
```sh
kubectl apply -f specs/
```

3. Verify application pod is running:
```sh
kubectl describe po app
```

4. Validate the pod successfully wrote data to the volume:
```sh
kubectl exec -it app -- cat /data/out.txt
```

5. Cleanup resources:
```sh
kubectl delete -f specs/
```
