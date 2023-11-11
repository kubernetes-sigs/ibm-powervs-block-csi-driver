# Integration Testing

## Introduction

The integration test will create a PowerVS instance and run the CSI driver tests on it. The driver will be built locally for ppc64le and copied over to the pvm instance. The driver will run on the instance and a SSH tunnel will be created. A grpc client will connect to the driver via a tcp port and run the Node and Controller tests.

You can also run the test on an existing PowerVS instance itself.

## Environments
The following environment variables will be required for running the integration test.
```
export IBMCLOUD_API_KEY=<api key>               # IBM Cloud API key
export POWERVS_ZONE="mon01",                  # IBM Cloud zone

```
Below environment variable is needed when running the tests within a PowerVS machine.
```
export POWERVS_INSTANCE_ID="<instance_id>"    # The pvm instance id where the test is running
```
Below environment variables are used if you want to use run tests from a remote machine.
```
export TEST_REMOTE_NODE="1"                   # Set to 1 if you want to run the remote node tests
export IBMCLOUD_ACCOUNT_ID="<account_id>"     # IBM Cloud account to use for creating the remote node
export POWERVS_CLOUD_INSTANCE_ID="<pid>"      # Workspace guid to use for creating the remote node
export POWERVS_NETWORK="pub-network"          # (Optional) The network to use for creating the remote node
export POWERVS_IMAGE="CentOS-Stream-8"        # (Optional) The image to use for creating the remote node
```


## Run the test
To run the test use the following command.
```
make test-integration
```
