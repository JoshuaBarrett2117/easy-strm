package main

import (
 "time"
 "easy-strm/internal/dao"
)

// GetCronTaskByID 复用统一业务层读取。
func GetCronTaskByID(id int)(*CronTask,error){return scheduler.GetByID(id)}
// GetCronTaskByName 复用统一业务层读取。
func GetCronTaskByName(name string)(*CronTask,error){return scheduler.GetByName(name)}
// GetCronTaskByStrmConfigID 查询配置的全量任务。
func GetCronTaskByStrmConfigID(id int)(*CronTask,error){t,e:=dao.NewCronTaskDAO().GetByStrmConfigID(id);if e!=nil||t==nil{return t,e};return scheduler.GetByID(t.ID)}
// GetAllCronTasks 返回统一任务配置。
func GetAllCronTasks()([]*CronTask,error){return scheduler.GetAll()}
// GetEnabledCronTasks 返回启用任务。
func GetEnabledCronTasks()([]*CronTask,error){return scheduler.GetEnabled()}
// CreateCronTask 复用统一保存与调度。
func CreateCronTask(name,kind string,cloud,config int,expr string)(*CronTask,error){return scheduler.Create(name,kind,cloud,config,expr)}
// UpdateCronTask 复用统一保存与调度。
func UpdateCronTask(id int,name,kind,expr,status string)(*CronTask,error){return scheduler.Update(id,name,kind,expr,status)}
// UpdateCronTaskRunInfo 保存执行摘要。
func UpdateCronTaskRunInfo(id int,last,next *time.Time,status,msg string)error{return scheduler.UpdateRunInfo(id,last,next,status,msg)}
// DeleteCronTask 删除任务及后续调度。
func DeleteCronTask(id int)error{return scheduler.Delete(id)}
// DeleteCronTaskByName 删除匹配任务。
func DeleteCronTaskByName(name string)error{t,e:=scheduler.GetByName(name);if e!=nil||t==nil{return e};return scheduler.Delete(t.ID)}
