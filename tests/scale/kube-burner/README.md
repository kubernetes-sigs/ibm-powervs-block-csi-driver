# Running Scale tests with kube burner
## Steps
- [Download](https://github.com/cloud-bulldozer/kube-burner/releases) kube burner binary
- Untar the tar file
```
tar -xvf kube-burner-0.15.1-Linux-ppc64le.tar.gz 
```
- Create the storage class 
```
kubectl apply -f tests/scale/kube-burner/static/sc.yaml
```
- Run workloads
```
./kube-burner init --uuid=3fs8d0dgdjknwf49s356g3 -c tests/scale/kube-burner/config.yaml
```