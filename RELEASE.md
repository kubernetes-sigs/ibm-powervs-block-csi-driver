# IBM PowerVS Block CSI Driver Release Process

## Choose the release version and release branch

1. Find the [latest release](https://github.com/kubernetes-sigs/ibm-powervs-block-csi-driver/releases/latest). For example, `v3.5.0`.
1. Increment the version according to semantic versioning https://semver.org/. For example, `v3.5.1` for bug fixes or updated images, `v3.6.0` for minor changes, `v4.0.0` for major changes.
1. Find or create the corresponding release branch that corresponds to a minor version. For example, for `v3.5.1` the release branch would be `release-3.5` and it would already exist. For `v3.6.0` it would be `release-3.6` and the branch would need to be created. If you do not have permission to create the branch, ask [OWNERS](OWNERS).

## Create the release commit

Checkout the release branch you chose above, for example `git checkout release-3.5`.

### Update README.md

1. Search for any references to the previous version on the README, and update them if necessary.
1. Update the [CSI Specification Compatibility Matrix](https://github.com/kubernetes-sigs/ibm-powervs-block-csi-driver#csi-specification-compatibility-matrix) section with the new version.

### Update kustomization.yaml

1. Edit the driver version in [kustomization.yaml](https://github.com/kubernetes-sigs/ibm-powervs-block-csi-driver/blob/main/deploy/kubernetes/overlays/stable/kustomization.yaml) with the release version.
1. Update any side car versions if required.

### Send a release PR to the release branch

At this point you should have all changes required for the release commit. Verify the changes via git diff and send a new [PR](https://github.com/kubernetes-sigs/ibm-powervs-block-csi-driver/pulls) with the commit against the release branch.


## Tag the release

Once the PR is merged tag the release commit with the relase tag. You can do this locally to push the tag or use GitHub to create the realease tag. You'll need push privileges for this step.

## Promote the new image on registry.k8s.io

1. Once you create a release the new image will appear at the [GCR repo](https://console.cloud.google.com/gcr/images/k8s-staging-cloud-provider-ibm/global/ibm-powervs-block-csi-driver).
1. Click on the release image and capture the `Digest` from overview tab. For example, `sha256:e9a1de089253f137c19d7414ac17c99ff7cc508473a8f12e0910c580dcc70a57`.
1. Create a PR to add the new release version with above image digest to the [registry.k8s.io/images/k8s-staging-cloud-provider-ibm/images.yaml](https://github.com/kubernetes/k8s.io/blob/main/registry.k8s.io/images/k8s-staging-cloud-provider-ibm/images.yaml)
1. Once the PR is merged, verify that the new image is listed at https://explore.ggcr.dev/?repo=registry.k8s.io%2Fcloud-provider-ibm%2Fibm-powervs-block-csi-driver


## Create the post-release commit

Checkout the main branch `git checkout main`.

### Update README.md

1. Update the [CSI Specification Compatibility Matrix](https://github.com/kubernetes-sigs/ibm-powervs-block-csi-driver#csi-specification-compatibility-matrix) section with the new version.

### Update kustomization.yaml

1. Edit the driver version in [kustomization.yaml](https://github.com/kubernetes-sigs/ibm-powervs-block-csi-driver/blob/main/deploy/kubernetes/overlays/stable/kustomization.yaml) with the release version.
1. Update any side car versions if required.

### Send a post-release PR to the main branch

Create a new [PR](https://github.com/kubernetes-sigs/ibm-powervs-block-csi-driver/pulls) with the commit against the main branch.

The README and kustomize deployment files must not be updated to refer to the new images until after the images have been verified, available, therefore it's necessary to make these changes in a post-release PR rather than the original release PR.
