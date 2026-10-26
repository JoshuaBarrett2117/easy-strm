package service

import (
	"context"
	"fmt"
	"testing"

	"easy-strm/internal/domain"
	driver "github.com/SheltonZhu/115driver/pkg/driver"
)

type shareRevisionStore struct {
	*strmMemory
	revision int
}

func (store *shareRevisionStore) GetStrmPlaybackEntry(ctx context.Context, id string) (domain.ShareStrmEntry, string, error) {
	entry, err := store.GetStrmEntry(ctx, id)
	return entry, fmt.Sprint(store.revision), err
}

func (store *shareRevisionStore) SaveStrmEntry(ctx context.Context, entry domain.ShareStrmEntry) error {
	store.revision++
	return store.strmMemory.SaveStrmEntry(ctx, entry)
}

func TestShareStrmDirectLinkMappingRevisionInvalidatesIdenticalPayload(t *testing.T) {
	service, memory, client := strmFixture(t)
	client.present = true
	store := &shareRevisionStore{strmMemory: memory, revision: 1}
	service.store = store
	calls := 0
	service.SetPlaybackDirectLink(func(context.Context, string, int, string, string) (string, error) {
		calls++
		return "https://cdn.example/video?t=4102444800", nil
	})
	if _, err := service.Playback(context.Background(), "entry", "Player/1"); err != nil {
		t.Fatal(err)
	}
	store.revision++
	if _, err := service.Playback(context.Background(), "entry", "Player/1"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("identical payload with new database lifecycle reused old URL")
	}
}

func TestShareStrmDirectLinkFileIDBackfillRefreshesRevision(t *testing.T) {
	service, memory, client := strmFixture(t)
	entry := memory.entries["entry"]
	entry.FileID = ""
	entry.FilePath = "Show.S02E03.mkv"
	memory.entries["entry"] = entry
	client.shareSnapByDir = map[string]*driver.ShareSnapResp{"0": mustUnmarshalShareSnap(t, `{"state":true,"data":{"count":1,"list":[{"fid":"file-3","n":"Show.S02E03.mkv","fc":1}]}}`)}
	store := &shareRevisionStore{strmMemory: memory, revision: 1}
	service.store = store
	for request := 0; request < 2; request++ {
		if _, err := service.Playback(context.Background(), "entry", "Player/1"); err != nil {
			t.Fatal(err)
		}
	}
	if store.revision != 2 || client.receives != 1 {
		t.Fatal("FileID backfill invalidated its own delivery identity")
	}
}
