# Release Process

The IBM PowerVS Block CSI Driver is released on an as-needed basis. The process is as follows:

1. An issue is proposing a new release with a changelog since the last release
2. All [OWNERS](OWNERS) must LGTM this release
3. An OWNER runs `git tag -s $VERSION` and inserts the changelog and pushes the tag with `git push $VERSION`
4. The release issue is closed
5. An announcement email is sent to `kubernetes-dev@googlegroups.com` with the subject `[ANNOUNCE] ibm-powervs-block-csi-driver $VERSION is released`
6. A new release will be cut when there are new releases in the following packages: `kubernetes, go, go builder, container-storage-interface, side cars`
7. A new patch release will be cut when there are minor package updates
8. A new release will be cut with new features
