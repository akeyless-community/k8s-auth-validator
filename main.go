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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func getKubeconfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	kubeconfig := filepath.Join(home, ".kube", "config")
	return kubeconfig, nil
}

type Options struct {
	Token             string `short:"t" long:"token" description:"Akeyless token" required:"true"`
	ApiGatewayUrl     string `short:"u" long:"api-gateway-url" description:"Akeyless API Gateway URL" required:"false" default:"https://api.akeyless.io"`
	GatewayNameFilter string `short:"g" long:"gateway-name-filter" description:"Akeyless Gateway Name Filter" required:"false"`
	Verbose           bool   `short:"v" long:"verbose" description:"Show verbose debug information"`
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

var token string
var timeout = 30000 * time.Millisecond
var listAllRunningGatewayKubeConfigs = make([]GatewayKubeAuthConfigs, 0)
var clientset *kubernetes.Clientset
var tokenReviewPayload TokenReviewPayload
var tokenReviewResponse TokenReviewResponse

var options Options

func main() {

	parser := flags.NewParser(&options, flags.HelpFlag|flags.PassDoubleDash)
	parser.NamespaceDelimiter = "-"

	twentythousandtonnesofcrudeoil.TheEnvironmentIsPerfectlySafe(parser, "AKEYLESS_")

	_, err := parser.Parse()
	handleError(parser, err)

	// Get kubeconfig path
	kubeconfig, err := getKubeconfigPath()
	if err != nil {
		fmt.Println("Error getting kubeconfig path:", err)
		os.Exit(1)
	}

	fmt.Println("Kubeconfig path:", kubeconfig)

	config, err := clientcmd.LoadFromFile(kubeconfig)
	if err != nil {
		fmt.Println("Error loading kubeconfig:", err)
		os.Exit(1)
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

	fmt.Println("Current context:", currentContext)
	fmt.Println("Cluster:", contextDetails.Cluster)
	fmt.Println("Namespace:", contextDetails.Namespace)
	fmt.Println("User:", contextDetails.AuthInfo)

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

	listGatewaysBody := akeyless.ListGateways{
		Token: &options.Token,
	}
	gatewayListResponse, _, err := client.ListGateways(context.Background()).Body(listGatewaysBody).Execute()
	if err != nil {
		panic(err.Error())
	}

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

	// loop through all the auth configs and compare the K8SHost property with the retrieved cluster endpoint of clusterDetails.Server
	for _, gatewayKubeAuthConfig := range listAllRunningGatewayKubeConfigs {
		for _, kubeAuthConfig := range gatewayKubeAuthConfig.KubeAuthConfigs.K8SAuths {
			if kubeAuthConfig.K8SHost == clusterDetails.Server {
				fmt.Println("Found matching K8S Auth Config for cluster:", kubeAuthConfig.K8SHost)
				fmt.Println("K8S Auth Config Name:", kubeAuthConfig.Name)
				fmt.Println("K8S Auth Config Access ID:", kubeAuthConfig.AuthMethodAccessID)

				if kubeAuthConfig.K8SCaCert != base64EncodedCertificateAuthorityData {
					fmt.Println("K8S Auth Config CA Cert does NOT match Kubernetes Auth Config Name:", aurora.BrightRed(kubeAuthConfig.Name))
				} else {
					fmt.Println("K8S Auth Config CA Cert matches the Kubernetes Auth Config Name:", aurora.BrightRed(kubeAuthConfig.Name))
				}

				// Validate Token Reviewer JWT Access
				tokenReviewResponse, err := lookupTokenReviewerStatus(kubeAuthConfig.K8SHost+"/apis/authentication.k8s.io/v1/tokenreviews", kubeAuthConfig)
				if err != nil {
					fmt.Println(err)
				}
				if tokenReviewResponse.Status.Authenticated {
					fmt.Println("Token Reviewer JWT Access is valid for user:", tokenReviewResponse.Status.User.Username)
				} else {
					fmt.Println("Token Reviewer JWT Access is NOT valid for user:", tokenReviewResponse.Status.User.Username)
				}
			}
		}
	}
}

func handleError(helpParser *flags.Parser, err error) {
	if err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			fmt.Println(err)
			os.Exit(0)
		} else {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
		}

		os.Exit(1)
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
			panic(err)
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
			fmt.Println("Cluster URL is not set")
		}

		k8sAuthConfigs := KubeAuthConfigs{
			K8SAuths: []KubeAuthConfig{},
		}
		return k8sAuthConfigs
	}
}

func lookupAllK8sAuthConfigsFromRunningGateways(listRunningGateways []akeyless.GwClusterIdentity) {
	var lookupThisGateway bool = true
	if options.GatewayNameFilter != "" {
		fmt.Println("Gateway Name Filter:", options.GatewayNameFilter)
	}
	for _, g := range listRunningGateways {
		if options.GatewayNameFilter != "" {
			lookupThisGateway = false

			var displayName = string(*g.DisplayName)
			var clusterName = string(*g.ClusterName)
			var usableClusterName string
			if len(displayName) > 0 {
				usableClusterName = displayName
			} else {
				usableClusterName = clusterName
			}

			if usableClusterName != "" {
				if options.Verbose {
					fmt.Println("Usable Cluster Name:", usableClusterName)
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
				lookupThisGateway = true
			} else {
				if options.Verbose {
					fmt.Println("Gateway Name Filter does NOT match so skipping gateway")
				}
				lookupThisGateway = false
			}
		}
		if lookupThisGateway {
			gwKubeAuthConfigs := lookupK8sAuthConfigs(g)
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
