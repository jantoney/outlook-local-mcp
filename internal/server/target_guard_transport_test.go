package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	kiotaauth "github.com/microsoft/kiota-abstractions-go/authentication"
	kiotahttp "github.com/microsoft/kiota-http-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	msgraphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
)

// serverTestRewriteMiddleware sends production-middleware output to httptest.
type serverTestRewriteMiddleware struct {
	baseURL string
}

// Intercept changes only scheme and host after Graph's `/me` replacement and
// URL-template encoding have completed.
func (middleware *serverTestRewriteMiddleware) Intercept(pipeline kiotahttp.Pipeline, index int, request *http.Request) (*http.Response, error) {
	request.URL.Scheme = "http"
	request.URL.Host = middleware.baseURL[len("http://"):]
	return pipeline.Next(request, index)
}

// newServerTestGraphClient creates the real generated SDK and production Graph
// middleware pipeline over a local HTTP server.
func newServerTestGraphClient(t *testing.T, handler http.Handler) (*msgraphsdk.GraphServiceClient, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	middlewares := msgraphcore.GetDefaultMiddlewaresWithOptions(nil)
	middlewares = append(middlewares, &serverTestRewriteMiddleware{baseURL: server.URL})
	httpClient := kiotahttp.GetDefaultClient(middlewares...)
	adapter, err := msgraphsdk.NewGraphRequestAdapterWithParseNodeFactoryAndSerializationWriterFactoryAndHttpClient(
		&kiotaauth.AnonymousAuthenticationProvider{}, nil, nil, httpClient,
	)
	if err != nil {
		server.Close()
		t.Fatalf("create Graph request adapter: %v", err)
	}
	return msgraphsdk.NewGraphServiceClient(adapter), server
}
