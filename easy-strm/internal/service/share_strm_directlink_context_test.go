package service

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"easy-strm/internal/domain"
)

type shareContextSettings struct {
	ShareTaskSettingsStore
	read func(context.Context, string) (*domain.SystemConfig, error)
}

func (settings *shareContextSettings) GetByKeyContext(ctx context.Context, key string) (*domain.SystemConfig, error) {
	return settings.read(ctx, key)
}

func TestShareStrmDirectLinkSnapshotLifecycleReadsCancel(t *testing.T) {
	for _, reader := range []string{"settings", "account", "default_directory"} {
		for _, snapshot := range []int32{1, 2, 3, 4} {
			t.Run(fmt.Sprintf("%s/snapshot_%d", reader, snapshot), func(t *testing.T) {
				service, _, client := strmFixture(t)
				client.present = true
				settings, account := service.settings, service.account
				if reader == "default_directory" {
					cfg, _ := service.Settings()
					cfg.TransferPath = ""
					if err := service.SaveSettings(cfg); err != nil {
						t.Fatal(err)
					}
				}
				var reads atomic.Int32
				entered, exited := make(chan context.Context, 1), make(chan struct{})
				gate := func(ctx context.Context) error {
					if reads.Add(1) != snapshot {
						return nil
					}
					entered <- ctx
					<-ctx.Done()
					close(exited)
					return ctx.Err()
				}
				service.SetPlaybackAccountReader(func(ctx context.Context, id int) (*domain.Cloud115, error) {
					if reader == "account" {
						if err := gate(ctx); err != nil {
							return nil, err
						}
					}
					return account(id)
				})
				service.settings = &shareContextSettings{ShareTaskSettingsStore: settings, read: func(ctx context.Context, key string) (*domain.SystemConfig, error) {
					if (reader == "settings" && key == shareStrmSettingsKey) || (reader == "default_directory" && key == ShareTransferDirectoryKey) {
						if err := gate(ctx); err != nil {
							return nil, err
						}
					}
					if key == ShareTransferDirectoryKey {
						return &domain.SystemConfig{ConfigVal: "/fixture"}, nil
					}
					return settings.GetByKey(key)
				}}
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				finished := make(chan error, 1)
				go func() { _, err := service.Playback(ctx, "entry", "Player/1"); finished <- err }()
				select {
				case actual := <-entered:
					if _, ok := actual.Deadline(); !ok {
						t.Error("snapshot reader has no lifetime bound")
					}
				case <-ctx.Done():
					t.Fatal("snapshot reader was not reached")
				}
				cancel()
				if !errors.Is(<-finished, context.Canceled) {
					t.Fatal("snapshot cancellation not delivered")
				}
				select {
				case <-exited:
				case <-time.After(3 * time.Second):
					t.Fatal("detached snapshot reader outlived cancellation")
				}
			})
		}
	}
}

func TestShareStrmDirectLinkConfigurationReadsCancel(t *testing.T) {
	for _, stage := range []string{"settings", "account", "default_directory"} {
		t.Run(stage, func(t *testing.T) {
			service, _, _ := strmFixture(t)
			if stage == "default_directory" {
				cfg, _ := service.Settings()
				cfg.TransferPath = ""
				if err := service.SaveSettings(cfg); err != nil {
					t.Fatal(err)
				}
			}
			entered := make(chan struct{})
			if stage == "account" {
				service.SetPlaybackAccountReader(func(ctx context.Context, _ int) (*domain.Cloud115, error) {
					close(entered)
					<-ctx.Done()
					return nil, ctx.Err()
				})
			} else {
				settings := service.settings
				service.settings = &shareContextSettings{ShareTaskSettingsStore: settings, read: func(ctx context.Context, key string) (*domain.SystemConfig, error) {
					if stage == "settings" || key == ShareTransferDirectoryKey {
						close(entered)
						<-ctx.Done()
						return nil, ctx.Err()
					}
					return settings.GetByKey(key)
				}}
			}
			ctx, cancel := context.WithCancel(context.Background())
			finished := make(chan error, 1)
			go func() { _, err := service.Playback(ctx, "entry", "Player/1"); finished <- err }()
			<-entered
			cancel()
			if !errors.Is(<-finished, context.Canceled) {
				t.Fatal("configuration read did not receive cancellation")
			}
		})
	}
}
