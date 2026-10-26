package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"testing"

	"easy-strm/internal/domain"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
)

func sharePlaybackPreparationCall(client *Client, operation string) error {
	switch operation {
	case "share_snap":
		_, err := client.GetShareSnap("fixture", "", "0")
		return err
	case "list":
		_, err := client.GetFileList(0, 0, 0, 100, 1, "UID=1;CID=2;SEID=3;KID=4")
		return err
	default:
		_, err := client.MkdirAll115("/fixture", 1, "UID=1;CID=2;SEID=3;KID=4")
		return err
	}
}

func TestSharePlaybackPreparationActualResponseMetadata(t *testing.T) {
	for _, operation := range []string{"share_snap", "list", "mkdir_list", "mkdir_create"} {
		for _, status := range []int{200, 403, 429, 503} {
			for _, errno := range []int{40101009, 990010, 990011, 911} {
				t.Run(fmt.Sprintf("%s/%d/%d", operation, status, errno), func(t *testing.T) {
					calls := 0
					client := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
						calls++
						responseStatus := status
						body := fmt.Sprintf(`{"state":false,"errno":%d,"error":"synthetic-private-body"}`, errno)
						if operation == "mkdir_create" && calls == 1 {
							responseStatus, body = 200, `{"state":true,"count":0,"data":[]}`
						}
						return &http.Response{StatusCode: responseStatus, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
					})}}
					err := sharePlaybackPreparationCall(client.WithPlaybackContext(context.Background()), operation)
					var upstream *domain.DirectLinkError
					if !errors.As(err, &upstream) || upstream.HTTPStatus != status || upstream.Errno != errno {
						t.Fatal("preparation lost actual SDK response metadata")
					}
					if status == 200 && errno == 40101009 && !errors.Is(err, driver.ErrPasswordIncorrect) {
						t.Fatal("preparation lost typed SDK password error")
					}
					if strings.Contains(err.Error(), "synthetic-private-body") {
						t.Fatal("preparation exposed raw SDK body")
					}
					wantCalls := 1
					if operation == "mkdir_create" {
						wantCalls = 2
					}
					if calls != wantCalls {
						t.Fatal("unexpected hidden SDK retry or missing mkdir")
					}
				})
			}
		}
	}
}

func TestSharePlaybackPreparationNetworkErrorAndUnknownMetadata(t *testing.T) {
	for _, operation := range []string{"share_snap", "list", "mkdir_list", "mkdir_create"} {
		for _, failure := range []string{"reset", "malformed"} {
			t.Run(operation+"/"+failure, func(t *testing.T) {
				calls := 0
				client := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
					calls++
					if operation == "mkdir_create" && calls == 1 {
						return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"state":true,"count":0,"data":[]}`))}, nil
					}
					if failure == "reset" {
						return nil, &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
					}
					return &http.Response{StatusCode: 502, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`synthetic-private-body`))}, nil
				})}}
				err := sharePlaybackPreparationCall(client.WithPlaybackContext(context.Background()), operation)
				var upstream *domain.DirectLinkError
				if !errors.As(err, &upstream) || upstream.Errno != 0 {
					t.Fatal("invented errno on transport/malformed failure")
				}
				if failure == "reset" {
					if upstream.HTTPStatus != 0 || !errors.Is(err, syscall.ECONNRESET) {
						t.Fatal("network chain lost or previous response status retained")
					}
				} else if upstream.HTTPStatus != 502 {
					t.Fatal("actual malformed-response HTTP status lost")
				}
				if strings.Contains(err.Error(), "synthetic-private-body") {
					t.Fatal("malformed SDK body exposed")
				}
			})
		}
	}
}

func TestSharePlaybackPreparationConcurrentMetadataIsolation(t *testing.T) {
	entered, release := make(chan struct{}, 2), make(chan struct{})
	client := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		entered <- struct{}{}
		<-release
		status, errno := 429, 911
		if request.URL.Query().Get("share_code") == "second" {
			status, errno = 403, 990011
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"state":false,"errno":%d}`, errno)))}, nil
	})}}
	finished := make(chan bool, 2)
	for _, code := range []string{"first", "second"} {
		go func() {
			_, err := client.WithPlaybackContext(context.Background()).GetShareSnap(code, "", "0")
			var upstream *domain.DirectLinkError
			valid := errors.As(err, &upstream)
			if valid && code == "first" {
				valid = upstream.HTTPStatus == 429 && upstream.Errno == 911
			} else if valid {
				valid = upstream.HTTPStatus == 403 && upstream.Errno == 990011
			}
			finished <- valid
		}()
	}
	<-entered
	<-entered
	close(release)
	for range 2 {
		if !<-finished {
			t.Fatal("concurrent preparation mixed response metadata")
		}
	}
}
