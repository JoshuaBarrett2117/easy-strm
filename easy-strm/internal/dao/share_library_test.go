package dao

import (
 "context"
 "strings"
 "testing"
 "easy-strm/internal/domain"
 "github.com/DATA-DOG/go-sqlmock"
)

func TestLibraryCombinedQuery(t *testing.T){
 database,mock,err:=sqlmock.New();if err!=nil{t.Fatal(err)};defer database.Close()
 low,high:=7.5,9.0
 q:=domain.ShareLibraryQuery{Keyword:"电影",TmdbID:100,MediaType:"movie",YearMin:2020,YearMax:2026,RatingMin:&low,RatingMax:&high,Genres:"18,35",Countries:"cn,us",Available:true,Sort:"rating",Direction:"desc",Page:2,PageSize:24}
 where,args:=libraryWhere(q)
 if !strings.Contains(where,"genre_ids &&") || !strings.Contains(where,"country_codes &&") || !strings.HasSuffix(where,"AND available") || len(args)!=9{t.Fatalf("错误的组合条件: %s %v",where,args)}
 mock.ExpectQuery(`WITH ranked AS[\s\S]*ORDER BY rating DESC NULLS LAST,work_key ASC LIMIT \$10 OFFSET \$11`).WithArgs("电影",100,"movie",2020,2026,low,high,"18,35","CN,US",24,24).WillReturnRows(sqlmock.NewRows([]string{"total","data"}).AddRow(25,`[{"work_key":"tmdb:movie:100"}]`))
 got,err:=NewShareRecordDAO(database).Library(context.Background(),q);if err!=nil || got.Total!=25{t.Fatalf("%+v %v",got,err)}
 if err=mock.ExpectationsWereMet();err!=nil{t.Fatal(err)}
}

func TestLibrarySourcesEmptyPage(t *testing.T){
 database,mock,err:=sqlmock.New();if err!=nil{t.Fatal(err)};defer database.Close()
 mock.ExpectQuery(`WITH sources AS`).WithArgs("tmdb:tv:1",20,40).WillReturnRows(sqlmock.NewRows([]string{"total","data"}).AddRow(1,`[]`))
 got,err:=NewShareRecordDAO(database).LibrarySources(context.Background(),"tmdb:tv:1",3,20);if err!=nil||got.Total!=1{t.Fatalf("%+v %v",got,err)}
 if err=mock.ExpectationsWereMet();err!=nil{t.Fatal(err)}
}
