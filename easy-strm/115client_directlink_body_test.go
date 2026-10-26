package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

type shareLinkBlockingBody struct {
	ctx             context.Context
	reading, closed chan struct{}
}

func (body *shareLinkBlockingBody) Read([]byte) (int, error) {
	close(body.reading)
	<-body.ctx.Done()
	return 0, body.ctx.Err()
}
func (body *shareLinkBlockingBody) Close() error { close(body.closed); return nil }

func TestShareDirectLinkSDKBodyKeepsContextUntilClosed(t *testing.T) {
	reading, closed := make(chan struct{}), make(chan struct{})
	var actual context.Context
	client := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		actual = request.Context()
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: &shareLinkBlockingBody{ctx: actual, reading: reading, closed: closed}}, nil
	})}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	finished := make(chan error, 1)
	go func() {
		_, err := client.GetFileDirectLinkContext(ctx, "fixture", "UID=1;CID=2;SEID=3;KID=4", "Lavf/fixture")
		finished <- err
	}()
	<-reading
	if actual.Err() != nil {
		t.Error("HTTP context canceled before response body finished")
	}
	cancel()
	if !errors.Is(<-finished, context.Canceled) {
		t.Fatal("body read cancellation lost")
	}
	<-closed
}
