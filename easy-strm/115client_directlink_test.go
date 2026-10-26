package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"easy-strm/internal/domain"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
)

func TestShareDirectLinkSDKCapturesActualResponseMetadata(t *testing.T) {
	for _, status := range []int{200, 403, 429, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.UserAgent() != "Lavf/fixture" {
					t.Error("upstream UA changed")
				}
				if request.Header.Get("X-Forwarded-For") != "" || request.Header.Get("X-Real-IP") != "" {
					t.Error("caller IP forwarded")
				}
				if deadline, ok := request.Context().Deadline(); !ok || time.Until(deadline) > 20*time.Second {
					t.Error("actual HTTP request has no deadline")
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"state":false,"errno":40101010,"error":"synthetic-private-body"}`))}, nil
			})}}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			_, err := client.GetFileDirectLinkContext(ctx, "fixture", "UID=1;CID=2;SEID=3;KID=4", "Lavf/fixture")
			var upstream *domain.DirectLinkError
			if !errors.As(err, &upstream) || upstream.HTTPStatus != status || upstream.Errno != 40101010 {
				t.Fatal("actual SDK response metadata was lost")
			}
			if status == 200 && !errors.Is(err, driver.ErrLoginTwoStepVerify) {
				t.Fatal("SDK error chain was lost")
			}
			if strings.Contains(err.Error(), "synthetic-private") {
				t.Fatal("SDK response body exposed")
			}
		})
	}
}

func TestShareDirectLinkSDKContextAndEffectiveUA(t *testing.T) {
	for _, ua := range []string{"", "Lavf/fixture", "Mozilla/fixture"} {
		t.Run(ua, func(t *testing.T) {
			entered := make(chan struct{})
			client := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.UserAgent() != ua {
					t.Error("effective upstream UA differs from signature key")
				}
				close(entered)
				<-request.Context().Done()
				return nil, request.Context().Err()
			})}}
			ctx, cancel := context.WithCancel(context.Background())
			finished := make(chan error, 1)
			go func() {
				_, err := client.GetFileDirectLinkContext(ctx, "fixture", "UID=1;CID=2;SEID=3;KID=4", ua)
				finished <- err
			}()
			<-entered
			cancel()
			if !errors.Is(<-finished, context.Canceled) {
				t.Fatal("cancel did not reach real SDK HTTP request")
			}
		})
	}
}

func TestSharePlaybackPreparationHTTPContext(t *testing.T) {
	for _, operation := range []string{"mkdir", "list", "receive", "share_snap"} {
		t.Run(operation, func(t *testing.T) {
			entered := make(chan struct{})
			client := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if _, ok := request.Context().Deadline(); !ok {
					t.Error("preparation request has no real deadline")
				}
				close(entered)
				<-request.Context().Done()
				return nil, request.Context().Err()
			})}}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			playback := client.WithPlaybackContext(ctx)
			finished := make(chan error, 1)
			go func() {
				var err error
				switch operation {
				case "mkdir":
					_, err = playback.MkdirAll115("/fixture", 1, "UID=1;CID=2;SEID=3;KID=4")
				case "list":
					_, err = playback.GetFileList(0, 0, 0, 100, 1, "UID=1;CID=2;SEID=3;KID=4")
				case "receive":
					err = playback.ReceiveShare("fixture", "", "1", "0", 1, "UID=1;CID=2;SEID=3;KID=4")
				case "share_snap":
					_, err = playback.GetShareSnap("fixture", "", "0")
				}
				finished <- err
			}()
			<-entered
			cancel()
			if !errors.Is(<-finished, context.Canceled) {
				t.Fatal("preparation request cancellation chain was lost")
			}
			if client.playbackContext != nil {
				t.Fatal("shared client was mutated")
			}
		})
	}
}
