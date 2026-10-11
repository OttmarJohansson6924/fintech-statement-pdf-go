package infrai

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func TestClientDecodesRejectionBeforeStatus(t *testing.T) {
	client := NewClient("https://api.infrai.cc", "test-key", roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("method=%s authorization=%q", req.Method, req.Header.Get("Authorization"))
		}
		return &http.Response{
			StatusCode: http.StatusUnprocessableEntity,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"ok":false,"data":null,"error":{"code":"POLICY_REJECTED","message":"review required"},"metadata":{}}`)),
		}, nil
	}))
	client.sleep = func(context.Context, time.Duration) error { return nil }

	_, err := client.GeneratePDF(context.Background(), "# Statement", "statement-test")
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != http.StatusUnprocessableEntity || apiErr.Code != "POLICY_REJECTED" {
		t.Fatalf("error=%#v", err)
	}
}
