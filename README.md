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
- `--verbose, -v`: Enables verbose logging to provide detailed debug information.

All arguments can be prefixed with "AKEYLESS_" when used as environment variables except with all dashes replaced with underscores.

### Kubeconfig

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

## Key Functions

- `getKubeconfigPath`: Returns the path to the kubeconfig file.
- `lookupK8sAuthConfigs`: Retrieves Kubernetes authentication configurations for a given Akeyless Gateway cluster.
- `lookupAllK8sAuthConfigsFromRunningGateways`: Retrieves Kubernetes authentication configurations for all running Akeyless Gateway clusters.
- `lookupTokenReviewerStatus`: Validates the Token Reviewer JWT Access for a given Kubernetes host and auth config.
- `handleError`: Handles errors during the command-line arguments parsing.

## Running the program

To run the program, compile it with Go and run the resulting binary with the appropriate command line arguments. For example:

```bash
go build main.go
./main --token YOUR_AKEYLESS_TOKEN
