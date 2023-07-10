package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	akeyless "github.com/akeylesslabs/akeyless-go/v2"
	"github.com/stretchr/testify/assert"
)

func TestRetrieveListOfGatewaysUsingToken(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.RequestURI {
		case "/v2/gateways":
			// return a successful response
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"clusters": [{"name": "test-gateway"}]}`))
		case "/v2/gateways?token=expired":
			// return a 401 error for expired tokens
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "token expired"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	// create a client with the URL of our mock server
	client := akeyless.NewAPIClient(&akeyless.Configuration{
		Servers: []akeyless.ServerConfiguration{
			{
				URL: mockServer.URL,
			},
		},
	}).V2Api

	t.Run("Successful call", func(t *testing.T) {
		gatewayListResponse := retrieveListOfGatewaysUsingToken(client, "valid-token")
		assert.NotNil(t, gatewayListResponse)
		assert.NotEmpty(t, gatewayListResponse.Clusters)
	})

	t.Run("Expired token", func(t *testing.T) {
		assert.PanicsWithValue(t, "Unable to to retrieve list of gateways with provided token:", func() {
			retrieveListOfGatewaysUsingToken(client, "expired")
		})
	})
}
