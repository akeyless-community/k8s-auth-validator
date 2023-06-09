package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	akeyless "github.com/akeylesslabs/akeyless-go/v2"
	"github.com/gojek/heimdall/httpclient"
	flags "github.com/jessevdk/go-flags"
	"github.com/logrusorgru/aurora/v4"
	"github.com/vito/twentythousandtonnesofcrudeoil"
	// "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Declare a variable to hold the exit function. In real code, this will call os.Exit.
var exitFunc = os.Exit

// mightExit would be part of your actual application code.
func mightExit(shouldExit bool, errorCode int) {
	if shouldExit {
		fmt.Println(aurora.BrightCyan("Exiting application..."))
		exitFunc(errorCode)
	}

	fmt.Println("The application continues...")
}

func getKubeconfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	kubeconfig := filepath.Join(home, ".kube", "config")
	return kubeconfig, nil
}

type Options struct {
	Token             string `short:"t" long:"token" description:"Akeyless token" required:"false"`
	ApiGatewayUrl     string `short:"u" long:"api-gateway-url" description:"Akeyless API Gateway URL" required:"false" default:"https://api.akeyless.io"`
	GatewayNameFilter string `short:"g" long:"gateway-name-filter" description:"Akeyless Gateway Name Filter" required:"false"`
	Verbose           bool   `short:"V" long:"verbose" description:"Show verbose debug information"`
	Version           bool   `short:"v" long:"version" description:"Print the version number and exit" required:"false"`
}

type KubeAuthConfig struct {
	Name                 string `json:"name,omitempty"`
	ID                   string `json:"id,omitempty"`
	ProtectionKey        string `json:"protection_key,omitempty"`
	AuthMethodAccessID   string `json:"auth_method_access_id,omitempty"`
	AuthMethodPrvKeyPem  string `json:"auth_method_prv_key_pem,omitempty"`
	AmTokenExpiration    int    `json:"am_token_expiration,omitempty"`
	K8SHost              string `json:"k8s_host,omitempty"`
	K8SCaCert            string `json:"k8s_ca_cert,omitempty"`
	K8STokenReviewerJwt  string `json:"k8s_token_reviewer_jwt,omitempty"`
	K8SIssuer            string `json:"k8s_issuer,omitempty"`
	K8SPubKeysPem        string `json:"k8s_pub_keys_pem,omitempty"`
	DisableIssValidation bool   `json:"disable_iss_validation,omitempty"`
	UseLocalCaJwt        bool   `json:"use_local_ca_jwt,omitempty"`
	ClusterAPIType       string `json:"cluster_api_type,omitempty"`
}

type KubeAuthConfigs struct {
	K8SAuths []KubeAuthConfig `json:"k8s_auths,omitempty"`
}

type GatewayKubeAuthConfigs struct {
	KubeAuthConfigs   KubeAuthConfigs
	GwClusterIdentity *akeyless.GwClusterIdentity
}

type TokenReviewPayload struct {
	Kind       string `json:"kind,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
	Spec       Spec   `json:"spec,omitempty"`
}

type TokenReviewResponse struct {
	Kind       string   `json:"kind,omitempty"`
	APIVersion string   `json:"apiVersion,omitempty"`
	Metadata   Metadata `json:"metadata,omitempty"`
	Spec       Spec     `json:"spec,omitempty"`
	Status     Status   `json:"status,omitempty"`
}
type Metadata struct {
	CreationTimestamp interface{} `json:"creationTimestamp,omitempty"`
}
type Spec struct {
	Token string `json:"token,omitempty"`
}
type Extra struct {
	AuthenticationKubernetesIoPodName []string `json:"authentication.kubernetes.io/pod-name,omitempty"`
	AuthenticationKubernetesIoPodUID  []string `json:"authentication.kubernetes.io/pod-uid,omitempty"`
}
type User struct {
	Username string   `json:"username,omitempty"`
	UID      string   `json:"uid,omitempty"`
	Groups   []string `json:"groups,omitempty"`
	Extra    Extra    `json:"extra,omitempty"`
}
type Status struct {
	Authenticated bool     `json:"authenticated,omitempty"`
	User          User     `json:"user,omitempty"`
	Audiences     []string `json:"audiences,omitempty"`
}

// Declare a new variable that will be set during the build process.
var version string
var commit string
var date string
var timeout = 30000 * time.Millisecond
var listAllRunningGatewayKubeConfigs = make([]GatewayKubeAuthConfigs, 0)
var tokenReviewResponse TokenReviewResponse

const GATEWAY_RUNNING_STATUS = "Running"
const EXIT_CODE_SUCCESS = 0
const EXIT_CODE_ERROR = 1

var options Options

func main() {

	parser := flags.NewParser(&options, flags.HelpFlag|flags.PassDoubleDash)
	parser.NamespaceDelimiter = "-"

	twentythousandtonnesofcrudeoil.TheEnvironmentIsPerfectlySafe(parser, "AKEYLESS_")

	_, err := parser.Parse()
	handleError(parser, err)

	if options.Version {
		fmt.Println("Version:", version)
		fmt.Println("Commit:", commit)
		fmt.Println("Date:", date)
		mightExit(true, EXIT_CODE_SUCCESS)
	}

	// error if token in not set
	if len(options.Token) == 0 {
		printErrorMessages("", "Akeyless token is not set. Please set the token using the -t or --token flag or set the AKEYLESS_TOKEN environment variable")
		mightExit(true, EXIT_CODE_ERROR)
	}

	// Get kubeconfig path
	kubeconfig, err := getKubeconfigPath()
	if err != nil {
		fmt.Println("Error getting kubeconfig path:", err)
		mightExit(true, EXIT_CODE_ERROR)
	}

	fmt.Println("Kubeconfig path:", aurora.BrightGreen(kubeconfig))

	config, err := clientcmd.LoadFromFile(kubeconfig)
	if err != nil {
		fmt.Println("Error loading kubeconfig:", err)
		mightExit(true, EXIT_CODE_ERROR)
	}

	// use the current context in kubeconfig
	// config2, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	// if err != nil {
	// 	panic(err.Error())
	// }

	// create the clientset
	// clientset, err := kubernetes.NewForConfig(config2)
	// if err != nil {
	// 	panic(err.Error())
	// }

	currentContext := config.CurrentContext
	contextDetails := config.Contexts[currentContext]
	clusterDetails := config.Clusters[contextDetails.Cluster]

	fmt.Println("Current context:", aurora.BrightGreen(currentContext))
	fmt.Println("Cluster:", aurora.BrightGreen(contextDetails.Cluster))
	fmt.Println("Namespace:", aurora.BrightGreen(contextDetails.Namespace))
	fmt.Println("User:", aurora.BrightGreen(contextDetails.AuthInfo))

	if len(options.GatewayNameFilter) > 0 {
		fmt.Println("Gateway Name Filter Flag Set:", aurora.BrightCyan(options.GatewayNameFilter))
	}

	if options.ApiGatewayUrl != "https://api.akeyless.io" && len(options.ApiGatewayUrl) > 0 {
		fmt.Println("Akeyless API Gateway URL Flag Set:", aurora.BrightCyan(options.ApiGatewayUrl))
	}

	// print verbose if flag is enabled
	if options.Verbose {
		fmt.Println("Verbose Flag Set:", aurora.BrightCyan(options.Verbose))
	}

	if options.ApiGatewayUrl == "" {
		printErrorMessages("", "Akeyless API Gateway URL is not set")
		mightExit(true, EXIT_CODE_ERROR)
	}

	base64EncodedCertificateAuthorityData := base64.StdEncoding.EncodeToString(clusterDetails.CertificateAuthorityData)
	if options.Verbose {
		fmt.Println("Certificate authority data:", base64EncodedCertificateAuthorityData)
		fmt.Println("Kubernetes Cluster Endpoint Url:", clusterDetails.Server)
	}

	// Initialize Akeyless client
	client := akeyless.NewAPIClient(&akeyless.Configuration{
		Servers: []akeyless.ServerConfiguration{
			{
				URL: options.ApiGatewayUrl,
			},
		},
	}).V2Api

	gatewayListResponse := retrieveListOfGatewaysUsingToken(client, options.Token)

	for _, gateway := range *gatewayListResponse.Clusters {

		// filter gateways by status so that only the status of "Running" are processed
		var gatewayStatus = string(*gateway.Status)

		if gatewayStatus != "Running" {
			continue
		} else {
			// We are in a running gateway cluster so let's gather all the k8s auth configs we can get
			var displayName = string(*gateway.DisplayName)
			var clusterName = string(*gateway.ClusterName)
			var usableClusterName string
			if len(displayName) > 0 {
				usableClusterName = displayName
			} else {
				usableClusterName = clusterName
			}

			if options.Verbose {
				fmt.Println("GW Cluster Usable Name:", usableClusterName)
			}

			// Check if the gateway cluster Url is set because we can only search for the cluster if it is set
			if gateway.ClusterUrl == nil {
				if options.Verbose {
					fmt.Println("Gateway cluster URL is not set")
				}
				continue
			} else {
				var clusterUrl = string(*gateway.ClusterUrl)
				if options.Verbose {
					fmt.Println("Gateway cluster URL:", clusterUrl)
				}
			}
		}
	}

	lookupAllK8sAuthConfigsFromRunningGateways(*gatewayListResponse.Clusters)

	foundAnyMatch := false

	// loop through all the auth configs and compare the K8SHost property with the retrieved cluster endpoint of clusterDetails.Server
	for _, gatewayKubeAuthConfig := range listAllRunningGatewayKubeConfigs {
		for _, kubeAuthConfig := range gatewayKubeAuthConfig.KubeAuthConfigs.K8SAuths {
			if kubeAuthConfig.K8SHost == clusterDetails.Server {
				foundAnyMatch = true
				fmt.Println()
				gatewayClusterName := string(*gatewayKubeAuthConfig.GwClusterIdentity.ClusterName)
				gatewayClusterDisplayName := string(*gatewayKubeAuthConfig.GwClusterIdentity.DisplayName)
				fmt.Println("Found matching K8S Auth Config for Gateway Cluster:", aurora.BrightGreen(gatewayClusterName))
				if len(gatewayClusterDisplayName) > 0 {
					fmt.Println("Gateway Cluster Display Name:", aurora.BrightGreen(gatewayClusterDisplayName))
				}
				fmt.Println("Found matching K8S Auth Config for kubernetes cluster:", aurora.BrightGreen(kubeAuthConfig.K8SHost))
				fmt.Println("K8S Auth Config Name:", aurora.BrightGreen(kubeAuthConfig.Name))
				fmt.Println("K8S Auth Config Access ID:", aurora.BrightGreen(kubeAuthConfig.AuthMethodAccessID))

				if kubeAuthConfig.K8SCaCert != base64EncodedCertificateAuthorityData {
					fmt.Println("K8S Auth Config CA Cert does NOT match Kubernetes Auth Config Name:", aurora.BrightRed(kubeAuthConfig.K8SCaCert))
				} else {
					fmt.Println("K8S Auth Config CA Cert matches the Kubernetes Auth Config Name:", aurora.BrightGreen("CA Cert matches"))
				}

				// Validate Token Reviewer JWT Access
				tokenReviewResponse, err := lookupTokenReviewerStatus(kubeAuthConfig.K8SHost+"/apis/authentication.k8s.io/v1/tokenreviews", kubeAuthConfig)
				if err != nil {
					fmt.Println(err)
				}
				if tokenReviewResponse.Status.Authenticated {
					fmt.Println("Token Reviewer JWT Access is valid for user:", aurora.BrightGreen(tokenReviewResponse.Status.User.Username))
				} else {
					fmt.Println("Token Reviewer JWT Access is NOT valid for user:", aurora.BrightRed(kubeAuthConfig.K8STokenReviewerJwt))
				}
			}
		}
	}

	if !foundAnyMatch {
		fmt.Println()
		printErrorMessages(clusterDetails.Server, "Unable to find any existing gateway k8s auth config with this kubernetes host endpoint:")
	}
}

func retrieveListOfGatewaysUsingToken(client *akeyless.V2ApiService, token string) akeyless.GatewaysListResponse {

	listGatewaysBody := akeyless.ListGateways{
		Token: &options.Token,
	}
	gatewayListResponse, _, err := client.ListGateways(context.Background()).Body(listGatewaysBody).Execute()
	if err != nil {
		printErrorMessages(err.Error(), "Unable to to retrieve list of gateways with provided token:")
		mightExit(true, EXIT_CODE_ERROR)
	}
	return gatewayListResponse
}

func printErrorMessages(context string, messages ...string) {
	fmt.Println(aurora.BrightRed("========================================================================================================================="))
	for _, msg := range messages {
		errorMessage := aurora.BrightRed(msg)
		if len(context) > 0 {
			fmt.Println(errorMessage, context)
		} else {
			fmt.Println(errorMessage)
		}
	}
	fmt.Println(aurora.BrightRed("========================================================================================================================="))
}

func handleError(helpParser *flags.Parser, err error) {
	if err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			fmt.Println(err)
			mightExit(true, EXIT_CODE_SUCCESS)
		} else {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
		}

		mightExit(true, EXIT_CODE_ERROR)
	}
}

func lookupK8sAuthConfigs(cluster akeyless.GwClusterIdentity) KubeAuthConfigs {

	_, isClusterUrlSet := cluster.GetClusterUrlOk()
	var k8sAuthConfigs KubeAuthConfigs

	if isClusterUrlSet {

		url := cluster.GetClusterUrl() + "/config/k8s-auths"

		// If verbose logging is enabled then print the url
		if options.Verbose {
			fmt.Println("Cluster URL with k8s auth path:", url)
		}

		httpRequestClient := httpclient.NewClient(httpclient.WithHTTPTimeout(timeout))

		// Create an http.Request instance
		req, _ := http.NewRequest(http.MethodGet, url, nil)

		bearerToken := "Bearer " + options.Token
		req.Header.Add("Authorization", bearerToken)
		// Call the `Do` method, which has a similar interface to the `http.Do` method
		res, err := httpRequestClient.Do(req)
		if err != nil {
			fmt.Println("Unable to get k8s auth configs:", cluster.GetClusterUrl(), err)
			return generateEmptyK8sAuthConfigs()
		}

		body, err := ioutil.ReadAll(res.Body)

		err2 := json.Unmarshal(body, &k8sAuthConfigs)
		if err2 != nil {
			fmt.Println(err)
		}

		// If verbose logging is enabled then print the k8s auth configs as json
		if options.Verbose {
			k8sAuthConfigsJson, _ := json.Marshal(k8sAuthConfigs)
			fmt.Println("K8s auth configs:", string(k8sAuthConfigsJson))
		}

		return k8sAuthConfigs
	} else {
		if options.Verbose {
			fmt.Println("Cluster URL is not set for ", cluster.GetClusterName())
		}

		return generateEmptyK8sAuthConfigs()
	}
}

func generateEmptyK8sAuthConfigs() KubeAuthConfigs {
	k8sAuthConfigs := KubeAuthConfigs{
		K8SAuths: []KubeAuthConfig{},
	}
	return k8sAuthConfigs
}

func afterLastSlash(s string) string {
	i := strings.LastIndex(s, "/")
	if i == -1 {
		// No slash found, return the entire string
		return s
	}
	// Return everything after the last slash
	return s[i+1:]
}

func lookupAllK8sAuthConfigsFromRunningGateways(listRunningGateways []akeyless.GwClusterIdentity) {
	var lookupThisGateway bool = true
	var clusterNameMatches bool = false
	var clusterUrlIsConfigured bool = false
	var clusterIsRunning bool = false

	for _, g := range listRunningGateways {

		if options.GatewayNameFilter != "" {
			lookupThisGateway = false

			var displayName = string(*g.DisplayName)
			var clusterName = string(*g.ClusterName)
			var shortClusterName = afterLastSlash(clusterName)
			var usableClusterName string
			DEFAULT_CLUSTER_NAME := "defaultCluster"
			if len(displayName) > 0 {
				usableClusterName = displayName
			} else if len(shortClusterName) > 0 && shortClusterName != DEFAULT_CLUSTER_NAME {
				usableClusterName = shortClusterName
			} else {
				usableClusterName = clusterName
			}

			if usableClusterName != "" {
				if options.Verbose {
					fmt.Println("Usable Cluster Name:", aurora.BrightYellow(usableClusterName))
				}
			} else {
				if options.Verbose {
					fmt.Println("Usable Cluster Name is empty so using full cluster name")
				}
				usableClusterName = *g.ClusterName
			}

			if strings.HasPrefix(usableClusterName, options.GatewayNameFilter) {
				if options.Verbose {
					fmt.Println("Gateway Name Filter matches so processing gateway")
				}
				clusterNameMatches = true
			} else {
				if options.Verbose {
					fmt.Println("Gateway Name Filter does NOT match so skipping gateway")
				}
				clusterNameMatches = false
			}
		} else {
			// If no gateway name filter is set then process all gateways
			clusterNameMatches = true
		}

		gClusterUrl, gClusterUrlIsSet := g.GetClusterUrlOk()

		if gClusterUrlIsSet {
			gClusterUrlString := string(*gClusterUrl)
			if len(gClusterUrlString) > 0 {
				if options.Verbose {
					fmt.Println("Gateway cluster URL is set so processing gateway:", aurora.BrightYellow(gClusterUrlString))
				}
				clusterUrlIsConfigured = true
			} else {
				if options.Verbose {
					fmt.Println("Gateway cluster URL is NOT set so skipping gateway")
				}
				clusterUrlIsConfigured = false
			}
		} else {
			if options.Verbose {
				fmt.Println("Gateway cluster URL is NOT set so skipping gateway")
			}
			clusterUrlIsConfigured = false
		}

		gStatus, gStatusIsSet := g.GetStatusOk()
		gStatusString := string(*gStatus)

		if gStatusIsSet && gStatusString != GATEWAY_RUNNING_STATUS {
			if options.Verbose {
				fmt.Println("Gateway Status is NOT 'Running':", aurora.BrightYellow(string(*g.Status)), aurora.BrightYellow(*g.ClusterName))
			}

		} else {
			if options.Verbose {
				fmt.Println("Gateway Status is 'Running':", aurora.BrightGreen(string(*g.Status)), aurora.BrightGreen(*g.ClusterName))
			}
			clusterIsRunning = true
		}

		// Only lookup the k8s auth configs if the cluster name matches, the cluster url is configured and the cluster is running
		lookupThisGateway = clusterNameMatches && clusterUrlIsConfigured && clusterIsRunning

		if lookupThisGateway {
			gwKubeAuthConfigs := lookupK8sAuthConfigs(g)
			// If there are any k8s auth configs then add them to the list
			if len(gwKubeAuthConfigs.K8SAuths) > 0 {
				gatewayKubeAuthConfigs := GatewayKubeAuthConfigs{
					GwClusterIdentity: &g,
					KubeAuthConfigs:   gwKubeAuthConfigs,
				}
				listAllRunningGatewayKubeConfigs = append(listAllRunningGatewayKubeConfigs, gatewayKubeAuthConfigs)
			}
		}
	}
}

func lookupTokenReviewerStatus(url string, kubeAuthConfig KubeAuthConfig) (TokenReviewResponse, error) {
	// Define a custom HTTP client with SSL check disabled.
	customClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	// Create a new HTTP client with a default timeout and the custom client.
	client := httpclient.NewClient(
		httpclient.WithHTTPTimeout(timeout),
		httpclient.WithHTTPClient(customClient),
	)

	var tokenReviewerJwt string = kubeAuthConfig.K8STokenReviewerJwt

	// Create an instance of the Spec struct.
	tokenReviewSpec := Spec{
		Token: tokenReviewerJwt,
	}
	// Create an instance of the Payload struct.
	payload := TokenReviewPayload{
		Kind:       "TokenReview",
		APIVersion: "authentication.k8s.io/v1",
		Spec:       tokenReviewSpec,
	}

	// Use the json.Marshal function to convert the Payload struct to JSON.
	payloadJson, err := json.Marshal(payload)
	if err != nil {
		fmt.Println(err)
	}

	// Convert payloadJson to io.Reader type
	payloadReader := bytes.NewBuffer(payloadJson)

	// Define the HTTP headers.
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")
	headers.Set("Authorization", "Bearer "+kubeAuthConfig.K8STokenReviewerJwt)

	// Make the POST request.
	response, err2 := client.Post(url, payloadReader, headers)
	if err2 != nil {
		fmt.Println(err2)
	}

	// deserialize the response body into a byte array
	body, err3 := ioutil.ReadAll(response.Body)
	if err3 != nil {
		fmt.Println(err3)
	}

	// deserialize the response body into a TokenReviewResponse struct
	err4 := json.Unmarshal(body, &tokenReviewResponse)
	if err4 != nil {
		fmt.Println(err4)
	}

	return tokenReviewResponse, nil
}
