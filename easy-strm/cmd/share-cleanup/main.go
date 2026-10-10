package main

import (
	"easy-strm/internal/dao"
	"flag"
	"fmt"
	"os"
)

func main() {
	options := dao.ShareCleanupOptions{}
	flag.StringVar(&options.Mode, "mode", "REBUILD", "仅支持 REBUILD，保留识别缓存、原始分享记录和全部配置")
	flag.StringVar(&options.Scope, "scope", "", "必须显式指定 all-share")
	flag.BoolVar(&options.Apply, "apply", false, "生成写入 SQL；工具始终不连接数据库或执行 SQL")
	flag.BoolVar(&options.ConfirmSelectionsDestroyed, "confirm-selections-destroyed", false, "确认销毁所有手动选择")
	flag.BoolVar(&options.Quiesced, "quiesced", false, "证明已停止分享任务与 Emby 扫描")
	flag.StringVar(&options.BackupAttestation, "backup-attestation", "", "备份审计编号（不要填写凭据或真实个人路径）")
	flag.StringVar(&options.Confirm, "confirm", "", "DESTROY_SHARE_SELECTIONS_AND_DERIVED_RECORDS")
	flag.Parse()
	script, err := dao.BuildShareCleanupSQL(options)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if options.Apply {
		fmt.Fprintln(os.Stderr, "警告：脚本会销毁全部分享选择与派生记录。仅生成 SQL，未执行。备份与停任务证明已提供。")
	}
	fmt.Print(script)
}
