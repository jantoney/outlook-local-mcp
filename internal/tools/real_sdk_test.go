package tools

import (
	"net/http"
	"net/http/httptest"
	"testing"

	kiotaauth "github.com/microsoft/kiota-abstractions-go/authentication"
	kiotahttp "github.com/microsoft/kiota-http-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	msgraphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
)

// realSDKRewriteMiddleware preserves the production Graph middleware's path
// substitutions and redirects only the final request host to a test server.
type realSDKRewriteMiddleware struct {
	baseURL string
}

// Intercept rewrites only scheme and host after production middleware runs.
func (middleware *realSDKRewriteMiddleware) Intercept(pipeline kiotahttp.Pipeline, middlewareIndex int, request *http.Request) (*http.Response, error) {
	request.URL.Scheme = "http"
	request.URL.Host = middleware.baseURL[len("http://"):]
	return pipeline.Next(request, middlewareIndex)
}

// newRealSDKGraphClient creates the generated SDK with production Graph
// middleware over a local transport, preserving exact /me route replacement.
func newRealSDKGraphClient(t *testing.T, handler http.Handler) (*msgraphsdk.GraphServiceClient, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	middlewares := msgraphcore.GetDefaultMiddlewaresWithOptions(nil)
	middlewares = append(middlewares, &realSDKRewriteMiddleware{baseURL: server.URL})
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
