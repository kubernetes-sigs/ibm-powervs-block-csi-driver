#FROM golang:1.15-buster AS builder
#WORKDIR /go/src/sigs.k8s.io/ibm-powervs-block-csi-driver
#COPY . .
#RUN make

FROM --platform=linux/ppc64le k8s.gcr.io/build-image/debian-base:v2.1.3 AS debian-base
RUN clean-install ca-certificates e2fsprogs mount udev util-linux xfsprogs
RUN clean-install bash multipath-tools sg3-utils
# COPY --from=builder /go/src/sigs.k8s.io/ibm-powervs-block-csi-driver/bin/ibm-powervs-block-csi-driver /bin/ibm-powervs-block-csi-driver
COPY ./bin/ibm-powervs-block-csi-driver /bin/ibm-powervs-block-csi-driver

ENTRYPOINT ["/bin/ibm-powervs-block-csi-driver"]

FROM --platform=linux/ppc64le registry.access.redhat.com/ubi8/ubi:8.5 AS rhel-base
RUN yum --disableplugin=subscription-manager -y install httpd php \
  && yum --disableplugin=subscription-manager clean all
RUN clean-install ca-certificates e2fsprogs mount udev util-linux xfsprogs
# COPY --from=builder /go/src/sigs.k8s.io/ibm-powervs-block-csi-driver/bin/ibm-powervs-block-csi-driver /bin/ibm-powervs-block-csi-driver
COPY ./bin/ibm-powervs-block-csi-driver /bin/ibm-powervs-block-csi-driver

ENTRYPOINT ["/bin/ibm-powervs-block-csi-driver"]