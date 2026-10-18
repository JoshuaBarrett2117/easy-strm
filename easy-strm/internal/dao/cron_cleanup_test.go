package dao

import (
	"easy-strm/internal/domain"
	"errors"
	"testing"
	"time"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestCleanupDefinitionTransaction(t *testing.T) {
	for _,fail:=range []bool{false,true} { t.Run(map[bool]string{false:"commit",true:"rollback"}[fail],func(t *testing.T){
		mock,close:=setupMockDB(t); defer close()
		task:=&domain.CronTask{Handler:"cleanup_directories",Params:map[string]interface{}{},CleanupTargets:[]domain.CronCleanupTarget{{Cloud115ID:1,DirPath:"/cache",DirCID:"untrusted",Enabled:true},{Cloud115ID:2,DirPath:"/cache",Enabled:true}}}
		mock.ExpectBegin()
		mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0,1))
		mock.ExpectQuery(`SELECT EXISTS.*t_cron_task`).WithArgs(0).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectQuery(`INSERT INTO t_cron_task\(`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(48))
		mock.ExpectExec(`DELETE FROM t_cron_cleanup_target`).WithArgs(48).WillReturnResult(sqlmock.NewResult(0,0))
		for i:=1;i<=2;i++ {
			expected:=mock.ExpectQuery(`INSERT INTO t_cron_cleanup_target`).WithArgs(48,i,"/cache",true)
			if fail && i==2 { expected.WillReturnError(errors.New("target insert failed")) } else { expected.WillReturnRows(sqlmock.NewRows([]string{"id","created","updated"}).AddRow(i,time.Now(),time.Now())) }
		}
		if fail { mock.ExpectRollback() } else { mock.ExpectCommit() }
		err:=NewCronTaskDAO().SaveDefinition(task)
		if (err!=nil)!=fail { t.Fatal(err) }
		if task.CleanupTargets[0].DirCID!="" { t.Fatal("CID persisted") }
		if err:=mock.ExpectationsWereMet();err!=nil {t.Fatal(err)}
	}) }
}
func TestCleanupSingleton(t *testing.T) {
	mock,close:=setupMockDB(t); defer close()
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0,1))
	mock.ExpectQuery(`SELECT EXISTS.*t_cron_task`).WithArgs(0).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()
	if err:=NewCronTaskDAO().SaveDefinition(&domain.CronTask{Handler:"cleanup_directories"});err==nil { t.Fatal("second singleton accepted") }
	if err:=mock.ExpectationsWereMet();err!=nil {t.Fatal(err)}
}
func TestCleanupReadAndAccountValidation(t *testing.T) {
	mock,close:=setupMockDB(t);defer close()
	mock.ExpectQuery(`SELECT EXISTS.*t_cloud_115`).WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	d:=NewCronTaskDAO()
	if err:=d.ValidateCleanupAccount(7);err==nil {t.Fatal("missing account")}
	mock.ExpectQuery(`SELECT id,cron_task_id.*FROM t_cron_cleanup_target`).WithArgs(48).WillReturnRows(sqlmock.NewRows([]string{"id","cron","account","path","cid","enabled","created","updated"}).AddRow(1,48,7,"/cache","",false,time.Now(),time.Now()))
	targets,err:=d.CleanupTargets(48)
	if err!=nil || len(targets)!=1 || targets[0].Enabled || targets[0].DirPath!="/cache" { t.Fatalf("%+v %v",targets,err) }
	if err:=mock.ExpectationsWereMet();err!=nil {t.Fatal(err)}
}
