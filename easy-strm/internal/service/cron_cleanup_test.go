package service

import (
	"context"
	"easy-strm/internal/domain"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type cleanupFake struct {
	children []CleanupEntry
	deleted []string
	offsets []int
	stuck bool
	failAccount int
	root []CleanupEntry
}
func (f *cleanupFake) List(ctx context.Context,a *domain.Cloud115,cid string,offset,limit int)([]CleanupEntry,error) {
	if err:=ctx.Err(); err!=nil { return nil,err }
	if a.ID==f.failAccount { return nil,errors.New("mock list failure") }
	if cid=="0" {
		if f.root!=nil { if offset>=len(f.root) { return nil,nil }; end:=offset+limit; if end>len(f.root) { end=len(f.root) }; return f.root[offset:end],nil }
		return []CleanupEntry{{ID:"10",ParentID:"0",Name:"cache",Directory:true}},nil
	}
	if cid!="10" { return nil,fmt.Errorf("unexpected CID %s",cid) }
	f.offsets=append(f.offsets,offset)
	end:=limit; if end>len(f.children) { end=len(f.children) }
	return append([]CleanupEntry(nil),f.children[:end]...),nil
}
func (f *cleanupFake) Delete(ctx context.Context,a *domain.Cloud115,ids []string)error {
	if len(ids)>100 { return errors.New("oversized delete") }
	f.deleted=append(f.deleted,ids...)
	if !f.stuck { f.children=f.children[len(ids):] }
	return ctx.Err()
}
func cleanupFixture(f *cleanupFake) *DirectoryCleanupService {
	s:=NewDirectoryCleanupService(f,func(id int)(*domain.Cloud115,error){ return &domain.Cloud115{ID:id,Name:fmt.Sprintf("account-%d",id)},nil })
	s.interval=0
	return s
}
func cleanupTask(ids ...int)*domain.CronTask {
	t:=&domain.CronTask{}
	for _,id:=range ids { t.CleanupTargets=append(t.CleanupTargets,domain.CronCleanupTarget{Cloud115ID:id,DirPath:"/cache",DirCID:"999",Enabled:true}) }
	return t
}
func TestCleanupTargetsValidation(t *testing.T) {
	for _,bad:=range []string{"","/","///","abc","../x","/a/../x","/x\\y","/x\n"} {
		targets:=[]domain.CronCleanupTarget{{Cloud115ID:1,DirPath:bad}}
		if err:=validateCleanupTargets(targets,func(int)error{ t.Fatal("invalid path queried account"); return nil }); err==nil { t.Errorf("accepted %q",bad) }
	}
	for _,pair:=range [][2]string{{"/a","/a/"},{"/a","/a/b"},{"/a/b","/a"}} {
		targets:=[]domain.CronCleanupTarget{{Cloud115ID:1,DirPath:pair[0]},{Cloud115ID:1,DirPath:pair[1]}}
		if err:=validateCleanupTargets(targets,func(int)error{return nil}); err==nil { t.Fatal(pair) }
	}
	targets:=cleanupTask(1,2).CleanupTargets
	if err:=validateCleanupTargets(targets,func(int)error{return nil}); err!=nil { t.Fatal(err) }
	if targets[0].DirCID!="" { t.Fatal("cached CID retained") }
	if err:=validateCleanupTargets(targets,func(int)error{return errors.New("missing account")}); err==nil { t.Fatal("missing account accepted") }
}
func TestCleanupPaginationRetainsDirectoryAndPartialFailure(t *testing.T) {
	f:=&cleanupFake{failAccount:2}
	for i:=0;i<237;i++ { f.children=append(f.children,CleanupEntry{ID:fmt.Sprint(100+i),ParentID:"10",Name:"child",Directory:i%2==0}) }
	s:=cleanupFixture(f)
	message,err:=s.Execute(context.Background(),cleanupTask(2,1),"")
	if err==nil { t.Fatal("partial failure must fail") }
	var results []struct { AccountID int `json:"cloud115_id"`; Count int `json:"deleted_count"`; Error string `json:"error"` }
	if e:=json.Unmarshal([]byte(message),&results); e!=nil { t.Fatal(e) }
	if len(results)!=2 || results[0].AccountID!=1 || results[0].Count!=237 || results[1].Error=="" { t.Fatal(message) }
	if len(f.children)!=0 || len(f.deleted)!=237 { t.Fatal("pagination skipped children") }
	for _,offset:=range f.offsets { if offset!=0 { t.Fatal("unsafe delete pagination") } }
	for _,id:=range f.deleted { if id=="10" || id=="999" { t.Fatal("deleted target or cached CID") } }
}
func TestCleanupFailureContinuesLaterAccount(t *testing.T) {
	f:=&cleanupFake{failAccount:1,children:[]CleanupEntry{{ID:"11",ParentID:"10"}}}
	message,err:=cleanupFixture(f).Execute(context.Background(),cleanupTask(1,2),"")
	if err==nil || len(f.deleted)!=1 || !strings.Contains(message,`"deleted_count":1`) { t.Fatalf("%s %v",message,err) }
}
func TestCleanupSafetyAndNoProgress(t *testing.T) {
	for _,entry:=range []CleanupEntry{{ID:"10",ParentID:"10"},{ID:"0",ParentID:"10"},{ID:"-1",ParentID:"10"},{ID:"garbage",ParentID:"10"},{ID:"11",ParentID:"other"}} {
		f:=&cleanupFake{children:[]CleanupEntry{entry}}
		_,err:=cleanupFixture(f).Execute(context.Background(),cleanupTask(1),"")
		if err==nil || len(f.deleted)!=0 { t.Fatalf("unsafe entry %+v: %v",entry,err) }
	}
	f:=&cleanupFake{stuck:true,children:[]CleanupEntry{{ID:"11",ParentID:"10"}}}
	message,err:=cleanupFixture(f).Execute(context.Background(),cleanupTask(1),"")
	if err==nil || len(f.deleted)!=3 || !strings.Contains(message,"未取得进展") { t.Fatalf("%s %v deletes=%d",message,err,len(f.deleted)) }
}
func TestCleanupResolvePaginationAndMissingDirectory(t *testing.T) {
	f:=&cleanupFake{}
	for i:=0;i<100;i++ { f.root=append(f.root,CleanupEntry{ID:fmt.Sprint(200+i),ParentID:"0",Name:"other",Directory:true}) }
	f.root=append(f.root,CleanupEntry{ID:"10",ParentID:"0",Name:"cache",Directory:true})
	if _,err:=cleanupFixture(f).Execute(context.Background(),cleanupTask(1),""); err!=nil { t.Fatal(err) }
	f.root=f.root[:100]
	if _,err:=cleanupFixture(f).Execute(context.Background(),cleanupTask(1),""); err==nil { t.Fatal("missing directory accepted") }
	if len(f.deleted)>0 { t.Fatal("missing directory deleted something") }
}
func TestCleanupCancellationAndTimeout(t *testing.T) {
	for _,timeout:=range []bool{false,true} {
		f:=&cleanupFake{}
		s:=cleanupFixture(f)
		ctx,cancel:=context.WithCancel(context.Background())
		if timeout { s.timeout=time.Nanosecond } else { cancel() }
		_,err:=s.Execute(ctx,cleanupTask(1,2),"")
		cancel()
		if err==nil || len(f.deleted)>0 { t.Fatalf("%v",err) }
	}
}
