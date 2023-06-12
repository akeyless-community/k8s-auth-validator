# Kubernetes auth Akeyless Validator

This Go CLI validates the configuration of the currently connected Kubernetes (k8s) clusters using Akeyless. 

It interacts with the Akeyless API Gateway and uses kubeconfig for Kubernetes interactions.

## Example

[![asciicast](https://asciinema.org/a/588498.svg)](https://asciinema.org/a/588498)

## Installation

### Homebrew install

```sh
brew install akeyless-community/kav/kav
```

### Other installation methods

Navigate to [the releases page](https://github.com/akeyless-community/k8s-auth-validator/releases) and find the correct release binary for your operating system and system architecture. Download the binary and make it executable to connect to the appropriate kubernetes cluster through kubectl.

## Running the CLI

### Running the CLI with token

```sh
k8s-auth-validator -t "t-432xxxxxxx234354grdsg443"
```

### Running the CLI with environment variable token

```sh
export AKEYLESS_TOKEN="t-432xxxxxxx234354grdsg443"
k8s-auth-validator
```

## Inputs

### Command Line Arguments

The program takes the following command line arguments:

- `--token, -t`: Akeyless token, required for making authenticated requests to the Akeyless API Gateway.
- `--api-gateway-url, -u`: The URL of the Akeyless API Gateway. By default, it is set to "https://api.akeyless.io".
- `--gateway-name-filter, -g`: A filter for the name of the Akeyless Gateway.
- `--verbose, -V`: Enables verbose logging to provide detailed debug information.
- `--version, -v`: Prints the version of the program and exits.

#### Token

The Akeyless `Token` is required for making authenticated requests to the Akeyless API Gateway. It can be obtained from the Akeyless Web Console or through the gateways web console.

#### API Gateway URL

The `API Gateway URL` can be used to connect to a local Akeyless Gateway API. This can be useful for single tenant deployments of Akeyless or for customers with customer fragments protecting their secrets.

#### Gateway Name Filter

Using the `Gateway Name Filter` can be useful for when you only want to focus on a single gateway and not loop through all the running gateway clusters. 

The `Gateway Name Filter` will first match against the Gateway Display Name and if no match is found, it will match against the Gateway Cluster name as long as the cluster name is not the default value of "defaultCluster", and if the flag is not set it will attempt to match against the full Gateway Name found within the Gateway screen of the Akeyless Web Console.

### Environment Variables

All arguments can be prefixed with "AKEYLESS_" when used as environment variables, simply replace the any remaining dashes with underscores.

```sh
#export AKEYLESS_TOKEN="t-23fds32432tg8wws23543"
export AKEYLESS_API_GATEWAY_URL="https://mylocalgateway.company.com:8081"
#export AKEYLESS_GATEWAY_NAME_FILTER="Gateway1-GKE"
#export AKEYLESS_GATEWAY_NAME_FILTER="acc-xf4cbk7dmj0kk/p-wyv8r36au41uy/Gateway1-GKE"
#export AKEYLESS_GATEWAY_NAME_FILTER="Gateway 1 in GKE"
#export AKEYLESS_VERBOSE="true"
```


## Kubeconfig

The program uses the kubeconfig file from the current user's home directory to interact with the Kubernetes cluster.

### Gateway and Kubernetes Configuration

The program retrieves the list of running gateways from the Akeyless API and their Kubernetes authentication configurations.

## Outputs

The program outputs several details about the configuration and status of the Kubernetes cluster and the Akeyless Gateways:

1. Path to the kubeconfig file.
2. Details about the current context, including the cluster name, namespace, and user.
3. Certificate authority data and Kubernetes Cluster Endpoint Url (if verbose logging is enabled).
4. Information about running Akeyless Gateway clusters.
5. If a matching Kubernetes authentication configuration is found for a cluster, the program prints the name and Access ID of the configuration.
6. If the Token Reviewer JWT Access is valid, it prints a message indicating so. If not, it prints a message indicating that it is not valid.

Any errors encountered during the execution of the program are also printed.
