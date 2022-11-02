# Running Scale tests with kube burner
## Steps
- Clone this repo and change the working directory to kube-burner
```
git clone https://github.com/kubernetes-sigs/ibm-powervs-block-csi-driver.git && cd ibm-powervs-block-csi-driver/tests/scale/kube-burner/
```
- Download latest kube burner binary from [here](https://github.com/cloud-bulldozer/kube-burner/releases)
```
wget https://github.com/cloud-bulldozer/kube-burner/releases/download/v0.17.1/kube-burner-0.17.1-Linux-ppc64le.tar.gz
```
- Untar the tar file
```
tar -xvf kube-burner-0.17.1-Linux-ppc64le.tar.gz 
```
- Create the storage class 
```
kubectl apply -f static/sc.yaml
```
- Run workloads
```
./kube-burner init --uuid=3fs8d0dgdjknwf49s356g3 -c config.yaml
```