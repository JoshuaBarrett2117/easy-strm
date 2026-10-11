package service

const (
	shareStrmSourceProgressUnit = "个来源文件记录（一个记录可映射多个季集）"
	shareStrmWorkProgressUnit   = "个作品待办（本次尝试消费）"
	shareStrmMetricCountNote    = "来源文件、作品待办、实际写入STRM和来源内季集去重属于不同单位；不能要求written + skipped_dedupe = processed_files，也不能据此推算存量STRM文件数。"
)

func shareStrmMetricMetadata(metadata map[string]interface{}) map[string]interface{} {
	if metadata["share_export"] != true {
		return metadata
	}
	result := make(map[string]interface{}, len(metadata)+15)
	for key, value := range metadata {
		result[key] = value
	}
	progressUnit := shareStrmSourceProgressUnit
	_, sourceQuery := metadata["export_query"]
	fullMetrics := metadata["full_export_completed"] == true && metadata["written"] != nil
	workProgress := metadata["processed_works"] != nil && metadata["phase"] != "preparing"
	if !fullMetrics && (workProgress || !sourceQuery && metadata["incremental"] == true) {
		progressUnit = shareStrmWorkProgressUnit
	}
	for _, key := range []string{"total_files", "processed_files", "success_files", "failed_files"} {
		result[key+"_unit"] = progressUnit
	}
	for _, key := range []string{"processed_works", "consumed_works", "round_consumed_works", "failed_works", "pending_works"} {
		result[key+"_unit"] = "个作品待办"
	}
	result["affected_sources_unit"] = shareStrmSourceProgressUnit
	result["skipped_sources_unit"] = shareStrmSourceProgressUnit
	result["exported_files_unit"] = shareStrmWrittenUnit
	result["consumption_written_unit"] = shareStrmWrittenUnit
	result["consumption_skipped_dedupe_unit"] = shareStrmSkippedDedupeUnit
	result["metric_count_note"] = shareStrmMetricCountNote
	return result
}
