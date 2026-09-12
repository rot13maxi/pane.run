package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/agent-surface/agent-surface/internal/awsstore"
	"github.com/agent-surface/agent-surface/internal/server"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type app struct{ store *awsstore.Store }
type recorder struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (r *recorder) Header() http.Header { return r.header }
func (r *recorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}
func (r *recorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(p)
}

func (a *app) handle(ctx context.Context, event events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	body := []byte(event.Body)
	if event.IsBase64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(event.Body)
		if err != nil {
			return events.APIGatewayV2HTTPResponse{}, err
		}
		body = decoded
	}
	path := event.RawPath
	if path == "" {
		path = event.RequestContext.HTTP.Path
	}
	target := path
	if event.RawQueryString != "" {
		target += "?" + event.RawQueryString
	}
	req, err := http.NewRequestWithContext(ctx, event.RequestContext.HTTP.Method, target, bytes.NewReader(body))
	if err != nil {
		return events.APIGatewayV2HTTPResponse{}, err
	}
	for k, v := range event.Headers {
		req.Header.Set(k, v)
	}
	req.URL = &url.URL{Path: path, RawQuery: event.RawQueryString}
	baseURL := "https://" + event.Headers["x-forwarded-host"]
	if event.Headers["x-forwarded-host"] == "" {
		baseURL = "https://" + event.RequestContext.DomainName
	}
	handler := server.NewHosted(a.store, baseURL, log.Default(), a.store)
	out := &recorder{header: make(http.Header)}
	handler.ServeHTTP(out, req)
	status := out.status
	if status == 0 {
		status = http.StatusOK
	}
	headers := map[string]string{}
	for k, v := range out.header {
		headers[k] = strings.Join(v, ",")
	}
	return events.APIGatewayV2HTTPResponse{StatusCode: status, Headers: headers, Body: out.body.String()}, nil
}

func required(name string) (string, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return "", errors.New(name + " is required")
	}
	return v, nil
}
func main() {
	table, err := required("PANE_TABLE")
	if err != nil {
		log.Fatal(err)
	}
	bucket, err := required("PANE_BUCKET")
	if err != nil {
		log.Fatal(err)
	}
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("load AWS config: %v", err)
	}
	st, err := awsstore.New(dynamodb.NewFromConfig(cfg), s3.NewFromConfig(cfg), table, bucket)
	if err != nil {
		log.Fatal(err)
	}
	application := &app{store: st}
	lambda.Start(application.handle)
}
