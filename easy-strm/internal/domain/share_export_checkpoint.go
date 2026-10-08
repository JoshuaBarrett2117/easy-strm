package domain

// ShareExportInput 保存同一数据库快照下的输出输入，不包含账号凭据。
type ShareExportInput struct {
	Settings                ShareStrmSettings
	Templates               map[string]string
	Categories              []*MediaCategory
	ConfigRevision          int64
	PreparedRevision        int64
	Fingerprint             string
	BaselineState           string
	ProtocolVersion         int
	LegacyOutputsReconciled bool
}

// ShareExportDirty 表示作品级待办；Revision 只用于本作品 CAS，不是水位。
type ShareExportDirty struct {
	WorkKey     string
	Revision    int64
	PendingKeys []string
}

// ShareExportSourceState 独立保存来源观察及输出关联，删除业务来源后仍保留。
type ShareExportSourceState struct {
	SourceFileID     int
	ShareID          int
	MediaID          int
	WorkKey          string
	FileVersion      int
	ShareVersion     int
	Available        bool
	Cancelled        bool
	EpisodeSignature string
	ExportKeys       []string
	State            string
}
