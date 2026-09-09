package service

import (
 "testing"
 "database/sql/driver"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "sync/atomic"
 "time"
 "easy-strm/internal/dao"
 "easy-strm/internal/domain"
 "github.com/DATA-DOG/go-sqlmock"
 "github.com/alicebob/miniredis/v2"
 "github.com/go-redis/redis/v8"
)

type enrichedResultArg struct{}
func(enrichedResultArg)Match(value driver.Value)bool{raw,ok:=value.([]byte);if !ok{return false};var r domain.TmdbIdentifyResult;if json.Unmarshal(raw,&r)!=nil{return false};return r.TmdbID==100 && r.VoteAverage!=nil && *r.VoteAverage==8.5 && len(r.Countries)==1 && r.Countries[0]=="CN"}

func TestHistoricalEnrichmentDeduplicatesAndChecksVersion(t *testing.T){
 database,mock,err:=sqlmock.New();if err!=nil{t.Fatal(err)};defer database.Close()
 mini:=miniredis.RunT(t);client:=redis.NewClient(&redis.Options{Addr:mini.Addr()});defer client.Close();dao.InitTaskRedisDAO(client)
 tasks:=NewTaskService(dao.NewTaskRedisDAO(client));var calls atomic.Int32
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){calls.Add(1);_=json.NewEncoder(w).Encode(map[string]interface{}{"vote_average":8.5,"genres":[]map[string]interface{}{{"id":18}},"origin_country":[]string{"CN"},"name":"作品","original_name":"作品","original_language":"zh","first_air_date":"2024-01-01"})}));defer server.Close()
 tmdb:=NewTmdbService("fixture",nil);tmdb.baseURL=server.URL;tmdb.httpClient=server.Client()
 s:=NewShareRecordService(dao.NewShareRecordDAO(database),tmdb,tasks,nil)
 mock.ExpectQuery(`SELECT count\(\*\) FROM t_share_media`).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(2))
 mock.ExpectQuery(`SELECT work_key FROM t_share_media`).WithArgs("").WillReturnRows(sqlmock.NewRows([]string{"key"}).AddRow("tmdb:tv:100"))
 raw:=`{"success":true,"tmdb_id":100,"media_type":"tv","title":"作品"}`
 mock.ExpectQuery(`SELECT id,version,file_name,result`).WithArgs("tmdb:tv:100").WillReturnRows(sqlmock.NewRows([]string{"id","version","file","result"}).AddRow(1,2,"第一集",raw).AddRow(2,5,"第二集",raw))
 mock.ExpectExec(`UPDATE t_share_media SET status=\$1,result=\$2,error=\$3,version=version\+1,updated_at=now\(\) WHERE id=\$4 AND version=\$5`).WithArgs("identified",enrichedResultArg{},"",1,2).WillReturnResult(sqlmock.NewResult(0,1))
 // 第二条已经人工修改，零行更新计入失败，不能覆盖新身份。
 mock.ExpectExec(`UPDATE t_share_media`).WithArgs("identified",enrichedResultArg{},"",2,5).WillReturnResult(sqlmock.NewResult(0,0))
 mock.ExpectQuery(`SELECT work_key FROM t_share_media`).WithArgs("tmdb:tv:100").WillReturnRows(sqlmock.NewRows([]string{"key"}))
 id,err:=s.StartLibraryEnrichment();if err!=nil{t.Fatal(err)}
 deadline:=time.Now().Add(3*time.Second);for time.Now().Before(deadline){if s.enrichMu.TryLock(){s.enrichMu.Unlock();break};time.Sleep(time.Millisecond)}
 task,err:=tasks.Get(id);if err!=nil{t.Fatal(err)}
 if task["status"]!="failed" || task["success_files"]!=float64(1) || task["failed_files"]!=float64(1){t.Fatalf("错误的补全结果: %+v",task)}
 if calls.Load()!=1{t.Fatalf("同一作品请求了%d次",calls.Load())}
 if err=mock.ExpectationsWereMet();err!=nil{t.Fatal(err)}
}

func TestLibraryMetadataMergePreservesExisting(t *testing.T){
 dst:=&domain.TmdbIdentifyResult{VoteAverage:float64Ptr(9),Countries:[]string{"JP"}}
 mergeLibraryMetadata(dst,&domain.TmdbIdentifyResult{VoteAverage:float64Ptr(8),GenreIDs:[]int{18}})
 if *dst.VoteAverage!=9 || dst.Countries[0]!="JP" || dst.GenreIDs[0]!=18{t.Fatalf("覆盖了已有数据: %+v",dst)}
 s:=&ShareRecordService{};if _,err:=s.StartLibraryEnrichment();err==nil{t.Fatal("依赖缺失不能创建任务")}
}
