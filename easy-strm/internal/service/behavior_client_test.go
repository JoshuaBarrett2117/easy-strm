package service

import (
	"context"
	"easy-strm/internal/domain"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func newBehaviorEventsTestClient(t *testing.T, handler http.HandlerFunc) *BehaviorHTTPClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/user/behavior/detail" || request.URL.Query().Get("limit") != "1000" {
			t.Errorf("unexpected behavior request: %s %s", request.Method, request.URL)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.Header.Get("Cookie") != "test-cookie" || request.Header.Get("Referer") != "https://115.com/" {
			t.Error("missing behavior request headers")
		}
		writer.Header().Set("Content-Type", "application/json")
		handler(writer, request)
	}))
	t.Cleanup(server.Close)
	client := NewBehaviorHTTPClient(server.Client())
	client.base = server.URL
	return client
}

func TestBehaviorHTTPEvents(t *testing.T) {
	const event = `{"id":"12345","type":22,"file_id":"23456","parent_id":"0","file_name":"example","file_category":0,"file_type":0,"file_size":0,"sha1":""}`
	wantEvent := domain.BehaviorEvent{ID: 12345, Type: 22, FileID: "23456", ParentID: "0", Name: "example"}
	cases := []struct {
		name  string
		body  string
		count int
		next  bool
		want  []domain.BehaviorEvent
	}{
		{"nested", `{"state":true,"code":0,"message":"","data":{"count":9886,"next_page":true,"list":[` + event + `]}}`, 9886, true, []domain.BehaviorEvent{wantEvent}},
		{"legacy_array", `{"state":true,"count":9886,"next_page":true,"data":[` + event + `]}`, 9886, true, []domain.BehaviorEvent{wantEvent}},
		{"nested_precedence", `{"state":true,"count":99,"next_page":false,"list":[{"id":999}],"data":{"count":9886,"next_page":true,"list":[` + event + `]}}`, 9886, true, []domain.BehaviorEvent{wantEvent}},
		{"nested_empty", `{"state":true,"count":99,"next_page":true,"list":[{"id":999}],"data":{"count":0,"next_page":false,"list":[]}}`, 0, false, []domain.BehaviorEvent{}},
		{"legacy_empty", `{"state":true,"count":0,"next_page":false,"data":[]}`, 0, false, []domain.BehaviorEvent{}},
		{"numeric_id_and_fields", `{"state":true,"data":{"count":1,"next_page":false,"list":[{"id":9007199254740993,"type":2,"file_id":"23456","parent_id":"34567","file_name":"movie.mkv","file_size":123456,"sha1":"test-sha1"}]}}`, 1, false, []domain.BehaviorEvent{{ID: 9007199254740993, Type: 2, FileID: "23456", ParentID: "34567", Name: "movie.mkv", Size: 123456, SHA1: "test-sha1"}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			client := newBehaviorEventsTestClient(t, func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Query().Get("offset") != "7" {
					t.Errorf("offset = %q, want 7", request.URL.Query().Get("offset"))
				}
				fmt.Fprint(writer, test.body)
			})
			page, err := client.Events(context.Background(), "test-cookie", 7)
			if err != nil {
				t.Fatal(err)
			}
			if !page.State || page.Count != test.count || page.NextPage != test.next || !reflect.DeepEqual(page.List, test.want) {
				t.Fatalf("page = %+v, want state=true count=%d next_page=%t list=%+v", page, test.count, test.next, test.want)
			}
		})
	}
}

func TestBehaviorHTTPEventsInvalidPayload(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"state_false", `{"state":false,"code":99,"message":"expired","data":{"count":0,"next_page":false,"list":[]}}`},
		{"state_false_with_legacy_data", `{"state":false,"data":[]}`},
		{"missing_data", `{"state":true,"count":0,"next_page":false,"list":[]}`},
		{"null_data", `{"state":true,"data":null}`},
		{"string_data", `{"state":true,"data":"invalid"}`},
		{"number_data", `{"state":true,"data":42}`},
		{"boolean_data", `{"state":true,"data":true}`},
		{"missing_list", `{"state":true,"data":{"count":0,"next_page":false}}`},
		{"null_list", `{"state":true,"data":{"list":null}}`},
		{"object_list", `{"state":true,"data":{"list":{}}}`},
		{"string_list", `{"state":true,"data":{"list":"invalid"}}`},
		{"null_event", `{"state":true,"data":{"list":[null]}}`},
		{"empty_event", `{"state":true,"data":{"list":[{}]}}`},
		{"scalar_event", `{"state":true,"data":{"list":[42]}}`},
		{"array_event", `{"state":true,"data":{"list":[[]]}}`},
		{"bad_event_id", `{"state":true,"data":{"list":[{"id":"not-an-id"}]}}`},
		{"overflow_event_id", `{"state":true,"data":{"list":[{"id":"9223372036854775808"}]}}`},
		{"fractional_event_id", `{"state":true,"data":{"list":[{"id":1.5}]}}`},
		{"null_event_id", `{"state":true,"data":{"list":[{"id":null}]}}`},
		{"missing_event_id", `{"state":true,"data":{"list":[{"type":22}]}}`},
		{"bad_event_type", `{"state":true,"data":{"list":[{"id":1,"type":"invalid"}]}}`},
		{"bad_file_id", `{"state":true,"data":{"list":[{"id":1,"file_id":{}}]}}`},
		{"bad_file_size", `{"state":true,"data":{"list":[{"id":1,"file_size":"invalid"}]}}`},
		{"bad_nested_count", `{"state":true,"count":0,"data":{"count":"invalid","list":[]}}`},
		{"bad_nested_next_page", `{"state":true,"next_page":false,"data":{"next_page":"invalid","list":[]}}`},
		{"no_fallback_to_outer_list", `{"state":true,"list":[],"data":{"list":[{"id":"invalid"}]}}`},
		{"legacy_null_event", `{"state":true,"data":[null]}`},
		{"legacy_bad_event", `{"state":true,"data":[{"id":"invalid"}]}`},
		{"legacy_bad_count", `{"state":true,"count":"invalid","data":[]}`},
		{"legacy_bad_next_page", `{"state":true,"next_page":"invalid","data":[]}`},
		{"invalid_json", `{"state":true,"data":{"list":[]}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			client := newBehaviorEventsTestClient(t, func(writer http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(writer, test.body)
			})
			page, err := client.Events(context.Background(), "test-cookie", 0)
			if err == nil {
				t.Fatalf("invalid payload accepted as page: %+v", page)
			}
			if strings.HasPrefix(test.name, "state_false") && !strings.Contains(err.Error(), "失败或Cookie失效") {
				t.Fatalf("state=false must fail through get(): %v", err)
			}
		})
	}
}

func TestBehaviorHTTPSelectBehaviorBatch(t *testing.T) {
	cases := []struct {
		name    string
		cursor  int64
		limit   int
		pages   []string
		offsets []int
		want    []domain.BehaviorID
		fetched int
		wantErr string
	}{
		{
			name: "cumulative_offsets_last_page_and_oldest_n", limit: 3,
			pages: []string{
				`{"count":9886,"next_page":true,"list":[{"id":90},{"id":80}]}`,
				`{"count":9886,"next_page":true,"list":[{"id":70},{"id":60},{"id":50}]}`,
				`{"count":9886,"next_page":false,"list":[{"id":40}]}`,
			},
			offsets: []int{0, 2, 5}, want: []domain.BehaviorID{40, 50, 60}, fetched: 6,
		},
		{
			name: "checkpoint_stops_before_next_page", cursor: 55, limit: 2,
			pages: []string{
				`{"count":9886,"next_page":true,"list":[{"id":90},{"id":80}]}`,
				`{"count":9886,"next_page":true,"list":[{"id":70},{"id":60},{"id":55},{"id":40}]}`,
			},
			offsets: []int{0, 2}, want: []domain.BehaviorID{60, 70}, fetched: 6,
		},
		{
			name: "empty_page_with_next_page_rejected", limit: 3,
			pages: []string{
				`{"count":9886,"next_page":true,"list":[{"id":90},{"id":80}]}`,
				`{"count":9886,"next_page":true,"list":[]}`,
			},
			offsets: []int{0, 2}, fetched: 2, wantErr: "行为分页为空但仍声明next_page",
		},
		{
			name: "empty_last_page", limit: 3,
			pages:   []string{`{"count":0,"next_page":false,"list":[]}`},
			offsets: []int{0},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var offsets []int
			client := newBehaviorEventsTestClient(t, func(writer http.ResponseWriter, request *http.Request) {
				offset, err := strconv.Atoi(request.URL.Query().Get("offset"))
				if err != nil {
					t.Errorf("invalid offset: %v", err)
				}
				position := len(offsets)
				offsets = append(offsets, offset)
				if position >= len(test.pages) {
					t.Error("requested page after termination")
					writer.WriteHeader(http.StatusBadRequest)
					return
				}
				fmt.Fprintf(writer, `{"state":true,"data":%s}`, test.pages[position])
			})
			var stats BehaviorStats
			batch, err := selectBehaviorBatch(context.Background(), client, "test-cookie", test.cursor, test.limit, &stats)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want %q", err, test.wantErr)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			var ids []domain.BehaviorID
			for _, event := range batch {
				ids = append(ids, event.ID)
			}
			if !reflect.DeepEqual(ids, test.want) || stats.Fetched != test.fetched || !reflect.DeepEqual(offsets, test.offsets) {
				t.Fatalf("ids=%v fetched=%d offsets=%v, want ids=%v fetched=%d offsets=%v", ids, stats.Fetched, offsets, test.want, test.fetched, test.offsets)
			}
		})
	}
}
