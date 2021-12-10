#FROM golang:1.15-buster AS builder
#WORKDIR /go/src/github.com/ppc64le-cloud/powervs-csi-driver
#COPY . .
#RUN make

FROM --platform=linux/ppc64le k8s.gcr.io/build-image/debian-base:v2.1.3 AS debian-base
RUN clean-install ca-certificates e2fsprogs mount udev util-linux xfsprogs
RUN clean-install bash multipath-tools sg3-utils
# COPY --from=builder /go/src/github.com/ppc64le-cloud/powervs-csi-driver/bin/powervs-csi-driver /bin/powervs-csi-driver
COPY ./bin/powervs-csi-driver /bin/powervs-csi-driver

ENTRYPOINT ["/bin/powervs-csi-driver"]

FROM --platform=linux/ppc64le registry.access.redhat.com/ubi8/ubi:8.5 AS rhel-base
RUN yum --disableplugin=subscription-manager -y install httpd php \
  && yum --disableplugin=subscription-manager clean all
RUN clean-install ca-certificates e2fsprogs mount udev util-linux xfsprogs
# COPY --from=builder /go/src/github.com/ppc64le-cloud/powervs-csi-driver/bin/powervs-csi-driver /bin/powervs-csi-driver
COPY ./bin/powervs-csi-driver /bin/powervs-csi-driver

ENTRYPOINT ["/bin/powervs-csi-driver"]