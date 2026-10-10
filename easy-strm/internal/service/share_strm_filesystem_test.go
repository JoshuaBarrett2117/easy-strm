package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func newStrmFilesystemFixture(t *testing.T, sources []domain.ShareStrmSource, dedupe bool) (*ShareStrmService, *strmMemory, domain.ShareStrmSettings) {
	t.Helper()
	store := &strmMemory{sources: sources, entries: map[string]domain.ShareStrmEntry{}}
	client := setupTaskRedisMock(t)
	t.Cleanup(func() { _ = client.Close() })
	service := NewShareStrmService(store, nil, nil, nil, &OrganizeService{}, NewTaskService(dao.NewTaskRedisDAO(client)), func() ([]*domain.MediaCategory, error) { return nil, nil }, nil, nil)
	cfg := domain.ShareStrmSettings{OutputPath: t.TempDir(), BaseURL: "https://media.example.test", DedupeExport: dedupe}
	return service, store, cfg
}

func filesystemSource(fileID, shareID int, size int64, episodes ...domain.ShareEpisode) domain.ShareStrmSource {
	return domain.ShareStrmSource{ID: fileID, MediaID: 8801, ShareID: shareID, Available: true, FileSize: size, ShareName: fmt.Sprintf("分享%d", shareID), WorkKey: "tmdb:tv:8801", URL: fmt.Sprintf("https://115.com/s/share%d", shareID), FileName: fmt.Sprintf("Show/版本%d.mkv", fileID), Episodes: episodes, Result: domain.TmdbIdentifyResult{Success: true, Title: "Show", Year: 2024, MediaType: "tv", TmdbID: 8801}}
}

func filesystemEpisode(season, episode int) domain.ShareEpisode {
	return domain.ShareEpisode{SeasonNumber: season, EpisodeNumber: episode}
}

func filesystemPath(season, episode int, suffix string) string {
	return fmt.Sprintf("tv/Show (2024) {tmdb-8801}/Season %02d/Show - S%02dE%02d%s.strm", season, season, episode, suffix)
}

func filesystemContent(source domain.ShareStrmSource) []byte {
	entryID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("share%d:path:%s", source.ShareID, source.FileName))).String()
	return []byte("https://media.example.test/share-strm/" + entryID + "\n")
}

func filesystemCollection(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		content, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = content
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func assertFilesystemCollection(t *testing.T, root string, want map[string][]byte) {
	t.Helper()
	if got := filesystemCollection(t, root); !reflect.DeepEqual(got, want) {
		t.Fatalf("磁盘路径及内容不一致：got=%q want=%q", got, want)
	}
}

func runFilesystemExport(t *testing.T, service *ShareStrmService, cfg domain.ShareStrmSettings, id string) (map[string]interface{}, error) {
	t.Helper()
	if err := service.tasks.Create(id, "strm_generate", "文件系统回归"); err != nil {
		t.Fatal(err)
	}
	err := service.export(context.Background(), cfg, domain.ShareLibraryQuery{}, id)
	task, taskErr := service.tasks.Get(id)
	if taskErr != nil {
		t.Fatal(taskErr)
	}
	return task["metadata"].(map[string]interface{}), err
}

func assertFilesystemMetadata(t *testing.T, metadata map[string]interface{}, written, skipped int) {
	t.Helper()
	if metadata["written_unit"] != shareStrmWrittenUnit || metadata["skipped_dedupe_unit"] != shareStrmSkippedDedupeUnit || metadata["conflict_policy"] != shareStrmConflictPolicy {
		t.Errorf("任务缺少中文计数单位或去重说明：%v", metadata)
	}
	for key, want := range map[string]int{"written": written, "exported_files": written, "skipped_dedupe": skipped} {
		if got := metadata[key]; got != float64(want) {
			raw, _ := json.Marshal(metadata)
			t.Errorf("%s=%v，期望%d；任务元数据=%s", key, got, want, raw)
		}
	}
}

// TestShareStrmFilesystemMixedEpisodeCounts 验证跨季部分胜出和全败来源按不同季集累计，重复映射不重复计数。
func TestShareStrmFilesystemMixedEpisodeCounts(t *testing.T) {
	first := filesystemSource(11001, 501, 300, filesystemEpisode(1, 1), filesystemEpisode(1, 2), filesystemEpisode(2, 1), filesystemEpisode(2, 2))
	winner := filesystemSource(11002, 502, 400, filesystemEpisode(1, 1), filesystemEpisode(2, 1))
	loser := filesystemSource(11003, 503, 100, filesystemEpisode(1, 1), filesystemEpisode(2, 1), filesystemEpisode(2, 1))
	service, _, cfg := newStrmFilesystemFixture(t, []domain.ShareStrmSource{first, winner, loser}, true)
	metadata, err := runFilesystemExport(t, service, cfg, "mixed")
	if err != nil {
		t.Fatal(err)
	}
	assertFilesystemCollection(t, cfg.OutputPath, map[string][]byte{
		filesystemPath(1, 1, ""): filesystemContent(winner),
		filesystemPath(1, 2, ""): filesystemContent(first),
		filesystemPath(2, 1, ""): filesystemContent(winner),
		filesystemPath(2, 2, ""): filesystemContent(first),
	})
	assertFilesystemMetadata(t, metadata, 4, 4)
}

// TestShareStrmFilesystemWinnerLevels 通过真实落盘内容验证全部选优层级及同分享后遇到大文件的行为。
func TestShareStrmFilesystemWinnerLevels(t *testing.T) {
	episode := filesystemEpisode(1, 1)
	for _, level := range []string{"availability", "size", "shareID", "fileID", "same-share-larger-first", "same-share-larger-later"} {
		t.Run(level, func(t *testing.T) {
			first := filesystemSource(11001, 501, 200, episode)
			second := filesystemSource(11002, 502, 200, episode)
			winner := first
			switch level {
			case "availability":
				first.Available, first.FileSize = false, 900
				winner = second
			case "size":
				second.FileSize = 300
				winner = second
			case "shareID":
				first.ShareID = 503
				first.URL = "https://115.com/s/share503"
				winner = second
			case "fileID":
				second.ShareID, second.URL = first.ShareID, first.URL
			case "same-share-larger-first":
				second.ShareID, second.URL = first.ShareID, first.URL
				first.FileSize = 300
				winner = first
			case "same-share-larger-later":
				second.ShareID, second.URL = first.ShareID, first.URL
				second.FileSize = 300
				winner = second
			}
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("reverse=%t", reverse), func(t *testing.T) {
					sources := []domain.ShareStrmSource{first, second}
					if reverse {
						sources[0], sources[1] = sources[1], sources[0]
					}
					service, _, cfg := newStrmFilesystemFixture(t, sources, true)
					metadata, err := runFilesystemExport(t, service, cfg, "winner")
					if err != nil {
						t.Fatal(err)
					}
					assertFilesystemCollection(t, cfg.OutputPath, map[string][]byte{filesystemPath(1, 1, ""): filesystemContent(winner)})
					assertFilesystemMetadata(t, metadata, 1, 1)
				})
			}
		})
	}
}

// TestShareStrmFilesystemLegacySuffixes 验证关闭去重时跨分享、同分享及同名分享保留精确旧后缀。
func TestShareStrmFilesystemLegacySuffixes(t *testing.T) {
	episode := filesystemEpisode(1, 1)
	for _, scenario := range []string{"cross-share", "same-share", "combined", "same-label"} {
		t.Run(scenario, func(t *testing.T) {
			first := filesystemSource(11001, 501, 200, episode)
			second := filesystemSource(11002, 502, 300, episode)
			sources := []domain.ShareStrmSource{first, second}
			want := map[string][]byte{}
			switch scenario {
			case "cross-share":
				want[filesystemPath(1, 1, "-分享501")] = filesystemContent(first)
				want[filesystemPath(1, 1, "-分享502")] = filesystemContent(second)
			case "same-share":
				second.ShareID, second.ShareName, second.URL = first.ShareID, first.ShareName, first.URL
				sources[1] = second
				want[filesystemPath(1, 1, "-文件11001")] = filesystemContent(first)
				want[filesystemPath(1, 1, "-文件11002")] = filesystemContent(second)
			case "combined":
				third := filesystemSource(11003, 501, 400, episode)
				sources = append(sources, third)
				want[filesystemPath(1, 1, "-分享501-文件11001")] = filesystemContent(first)
				want[filesystemPath(1, 1, "-分享502")] = filesystemContent(second)
				want[filesystemPath(1, 1, "-分享501-文件11003")] = filesystemContent(third)
			case "same-label":
				first.ShareName, second.ShareName = "分享合集", "分享合集"
				sources = []domain.ShareStrmSource{first, second}
				want[filesystemPath(1, 1, "-分享合集-分享501")] = filesystemContent(first)
				want[filesystemPath(1, 1, "-分享合集-分享502")] = filesystemContent(second)
			}
			service, _, cfg := newStrmFilesystemFixture(t, sources, false)
			metadata, err := runFilesystemExport(t, service, cfg, "legacy")
			if err != nil {
				t.Fatal(err)
			}
			assertFilesystemCollection(t, cfg.OutputPath, want)
			assertFilesystemMetadata(t, metadata, len(want), 0)
		})
	}
}

// TestShareStrmFilesystemRepeatableCollection 比较新目录和原目录重复导出的全部相对路径及文件字节。
func TestShareStrmFilesystemRepeatableCollection(t *testing.T) {
	for _, dedupe := range []bool{true, false} {
		t.Run(fmt.Sprintf("dedupe=%t", dedupe), func(t *testing.T) {
			sources := []domain.ShareStrmSource{
				filesystemSource(11001, 501, 200, filesystemEpisode(1, 1), filesystemEpisode(2, 3)),
				filesystemSource(11002, 502, 300, filesystemEpisode(1, 1)),
				filesystemSource(11003, 501, 400, filesystemEpisode(2, 3)),
			}
			service, _, cfg := newStrmFilesystemFixture(t, sources, dedupe)
			if _, err := runFilesystemExport(t, service, cfg, "initial"); err != nil {
				t.Fatal(err)
			}
			before := filesystemCollection(t, cfg.OutputPath)
			if _, err := runFilesystemExport(t, service, cfg, "rerun"); err != nil {
				t.Fatal(err)
			}
			assertFilesystemCollection(t, cfg.OutputPath, before)
			cfg.OutputPath = t.TempDir()
			if _, err := runFilesystemExport(t, service, cfg, "fresh"); err != nil {
				t.Fatal(err)
			}
			assertFilesystemCollection(t, cfg.OutputPath, before)
		})
	}
}

type filesystemFaultStore struct {
	*strmMemory
	registrations int
	failAt        int
	failure       error
}

// SaveExportedStrmFile 在指定次数模拟清单登记失败，不影响已完成的真实文件写入。
func (store *filesystemFaultStore) SaveExportedStrmFile(ctx context.Context, file domain.StrmFile) error {
	store.registrations++
	if store.registrations == store.failAt {
		return store.failure
	}
	return store.strmMemory.SaveExportedStrmFile(ctx, file)
}

// TestShareStrmFilesystemPartialFailureCounts 验证写盘、清单登记及后续季集校验失败均保留已写文件和去重计数。
func TestShareStrmFilesystemPartialFailureCounts(t *testing.T) {
	for _, failure := range []string{"write", "registration", "invalid-episode"} {
		for _, full := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/full=%t", failure, full), func(t *testing.T) {
				first := filesystemSource(11001, 501, 200, filesystemEpisode(1, 1), filesystemEpisode(1, 1), filesystemEpisode(1, 2), filesystemEpisode(1, 3), filesystemEpisode(2, 1))
				winner := filesystemSource(11002, 502, 300, filesystemEpisode(1, 1))
				service, memory, cfg := newStrmFilesystemFixture(t, []domain.ShareStrmSource{first, winner}, true)
				want := map[string][]byte{filesystemPath(1, 2, ""): filesystemContent(first), filesystemPath(1, 3, ""): filesystemContent(first)}
				registrationErr := errors.New("文件清单登记故障")
				switch failure {
				case "write":
					blocker := "tv/Show (2024) {tmdb-8801}/Season 02"
					if err := os.MkdirAll(filepath.Dir(filepath.Join(cfg.OutputPath, blocker)), 0755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(cfg.OutputPath, blocker), []byte("保留阻挡文件"), 0644); err != nil {
						t.Fatal(err)
					}
					want[blocker] = []byte("保留阻挡文件")
				case "registration":
					service.store = &filesystemFaultStore{strmMemory: memory, failAt: 2, failure: registrationErr}
				case "invalid-episode":
					first.Episodes[len(first.Episodes)-1] = filesystemEpisode(2, 0)
					memory.sources[0] = first
				}
				if full {
					metadata, err := runFilesystemExport(t, service, cfg, "partial")
					if err == nil || !strings.Contains(err.Error(), "已写入3个STRM文件，1个本地记录失败") {
						t.Fatalf("失败摘要未保留文件计数：%v", err)
					}
					assertFilesystemMetadata(t, metadata, 3, 1)
					want[filesystemPath(1, 1, "")] = filesystemContent(winner)
				} else {
					conflicts, err := service.shareStrmConflicts(context.Background(), domain.ShareLibraryQuery{})
					if err != nil {
						t.Fatal(err)
					}
					result := service.exportLocalStrm(context.Background(), cfg, first, nil, map[string]bool{}, conflicts)
					if result.Err == nil || result.Written != 2 || result.SkippedDedupe != 1 {
						t.Fatalf("失败未保留部分计数：%+v", result)
					}
					if failure == "registration" && !errors.Is(result.Err, registrationErr) {
						t.Fatalf("登记错误丢失：%v", result.Err)
					}
				}
				assertFilesystemCollection(t, cfg.OutputPath, want)
			})
		}
	}
}

// TestShareStrmFilesystemDuplicateWinnerEpisodes 验证重复季集不多写、不多计，并保留旧模式的后缀。
func TestShareStrmFilesystemDuplicateWinnerEpisodes(t *testing.T) {
	source := filesystemSource(11001, 501, 300, filesystemEpisode(1, 1), filesystemEpisode(1, 1), filesystemEpisode(2, 1))
	for _, dedupe := range []bool{true, false} {
		t.Run(fmt.Sprintf("dedupe=%t", dedupe), func(t *testing.T) {
			service, _, cfg := newStrmFilesystemFixture(t, []domain.ShareStrmSource{source}, dedupe)
			metadata, err := runFilesystemExport(t, service, cfg, "duplicate-entries")
			if err != nil {
				t.Fatal(err)
			}
			suffix := ""
			if !dedupe {
				suffix = "-文件11001"
			}
			assertFilesystemCollection(t, cfg.OutputPath, map[string][]byte{filesystemPath(1, 1, suffix): filesystemContent(source), filesystemPath(2, 1, ""): filesystemContent(source)})
			assertFilesystemMetadata(t, metadata, 2, 0)
		})
	}
}

// TestShareStrmFilesystemOutputRegistrationFailure 验证安全输出落盘后登记失败仍计数，未改写的旧文件不计数。
func TestShareStrmFilesystemOutputRegistrationFailure(t *testing.T) {
	for _, unchanged := range []bool{false, true} {
		t.Run(fmt.Sprintf("unchanged=%t", unchanged), func(t *testing.T) {
			source := filesystemSource(11001, 501, 200, filesystemEpisode(1, 1))
			service, _, cfg := newStrmFilesystemFixture(t, []domain.ShareStrmSource{source}, true)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			connection, err := db.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = connection.Close() })
			localPath := filepath.Join(cfg.OutputPath, filesystemPath(1, 1, ""))
			rows := sqlmock.NewRows([]string{"owner_key", "export_key"})
			if unchanged {
				if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(localPath, filesystemContent(source), 0644); err != nil {
					t.Fatal(err)
				}
				rows.AddRow("share:default", "8801:1:1")
			}
			for range 2 {
				mock.ExpectQuery(`SELECT pg_try_advisory_lock\(`).WithArgs(localPath).WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
			}
			mock.ExpectQuery("SELECT owner_key,export_key").WithArgs(localPath).WillReturnRows(rows)
			mock.ExpectExec("INSERT INTO t_strm_export_history VALUES").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectBegin()
			mock.ExpectExec("INSERT INTO t_strm_export_history SELECT").WillReturnResult(sqlmock.NewResult(0, 1))
			registrationErr := errors.New("安全输出登记故障")
			mock.ExpectExec("INSERT INTO t_strm_export_state").WillReturnError(registrationErr)
			mock.ExpectRollback()
			for range 2 {
				mock.ExpectExec(`SELECT pg_advisory_unlock\(`).WithArgs(localPath).WillReturnResult(sqlmock.NewResult(0, 0))
			}
			output := &StrmOutput{Store: &dao.StrmExportDAO{Conn: connection}, Root: cfg.OutputPath, Owner: "share:default", Run: "registration-failure", Paths: map[string]bool{}}
			ctx := context.WithValue(context.Background(), shareExportOutputContext{}, output)
			result := service.exportLocalStrm(ctx, cfg, source, nil, map[string]bool{})
			written := 1
			if unchanged {
				written = 0
			}
			if !errors.Is(result.Err, registrationErr) || result.Written != written || result.SkippedDedupe != 0 {
				t.Fatalf("安全输出登记失败计数不正确：%+v", result)
			}
			assertFilesystemCollection(t, cfg.OutputPath, map[string][]byte{filesystemPath(1, 1, ""): filesystemContent(source)})
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type filesystemReader struct{ *strmMemory }

// GetStrmSource 仅从内存重新读取测试来源，不访问任何外部服务。
func (store filesystemReader) GetStrmSource(_ context.Context, id int) (domain.ShareStrmSource, error) {
	for _, source := range store.sources {
		if source.ID == id {
			return source, nil
		}
	}
	return domain.ShareStrmSource{}, errors.New("测试来源不存在")
}
