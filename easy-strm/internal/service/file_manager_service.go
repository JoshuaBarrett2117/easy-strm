package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/google/uuid"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

const fileManagerTaskType = "file_transfer"

const cloud115OperationRetryCount = 3

var cloud115RetryBaseDelay = time.Second

type fileManagerMediaSourceRepository interface {
	GetAll(sortField, sortOrder string) ([]*domain.MediaSource, error)
	GetByID(id int) (*domain.MediaSource, error)
}

type fileManagerCloud115Repository interface {
	GetAll(sortField, sortOrder string) ([]*domain.Cloud115, error)
	GetByID(id int) (*domain.Cloud115, error)
}

type fileManagerTaskManager interface {
	Create(taskID string, taskType, taskName string) error
	UpdateStatus(taskID, status string) error
	UpdateProgress(taskID string, totalFiles, processedFiles, successFiles, failedFiles int) error
	UpdateMetadata(taskID string, metadata map[string]interface{}) error
	SetError(taskID, errMsg string) error
	IsCancelled(taskID string) bool
	RegisterCancel(taskID string, cancel context.CancelFunc)
	RemoveCancel(taskID string)
}

// FileManagerCloudClient 定义文件管理器需要的115能力，便于在服务测试中替换。
type FileManagerCloudClient interface {
	GetFileList(cid int, showDir int, offset int, limit int, cloud115ID int, cookie string) (*driver.FileListResp, error)
	CopyFile(fileID, targetDirID string, cloud115ID int, cookie string) error
	MoveFile115(fileID, targetDirID string, cloud115ID int, cookie string) error
	RapidTransferFileByMetadata(sourceFileID, sourcePickCode, sourceSHA1 string, sourceSize int64, sourceCloud115ID int, sourceCookie string, targetDirID string, targetCloud115ID int, targetCookie string, fileName string) (string, error)
	CreateDirectory115(parentID, name string, cloud115ID int, cookie string) (string, error)
	DeleteFiles115(fileIDs []string, cloud115ID int, cookie string) error
	UploadLocalFile115(filePath, targetDirID, fileName string, cloud115ID int, cookie string) error
	DownloadFile115(pickCode, targetPath string, cloud115ID int, cookie string) error
}

// FileManagerService 编排本地与115位置之间的文件浏览和传输。
type FileManagerService struct {
	mediaSources fileManagerMediaSourceRepository
	cloud115     fileManagerCloud115Repository
	cloudClient  FileManagerCloudClient
	tasks        fileManagerTaskManager
}

// NewFileManagerService 创建统一文件管理服务。
func NewFileManagerService(mediaSources fileManagerMediaSourceRepository, cloud115 fileManagerCloud115Repository, cloudClient FileManagerCloudClient, tasks fileManagerTaskManager) *FileManagerService {
	return &FileManagerService{mediaSources: mediaSources, cloud115: cloud115, cloudClient: cloudClient, tasks: tasks}
}

// ListLocations 返回所有本地媒体源和所有115账号。
func (s *FileManagerService) ListLocations() ([]domain.FileManagerLocation, error) {
	sources, err := s.mediaSources.GetAll("", "")
	if err != nil {
		return nil, fmt.Errorf("查询本地媒体源失败: %w", err)
	}
	accounts, err := s.cloud115.GetAll("", "")
	if err != nil {
		return nil, fmt.Errorf("查询115账号失败: %w", err)
	}

	locations := make([]domain.FileManagerLocation, 0, len(sources)+len(accounts))
	for _, source := range sources {
		if source.SourceType != domain.SourceTypeLocal {
			continue
		}
		status := "disabled"
		if source.Enabled {
			status = "active"
		}
		locations = append(locations, domain.FileManagerLocation{Type: domain.FileManagerLocationLocal, ID: source.ID, Name: source.Name, Root: source.Path, Status: status})
	}
	for _, account := range accounts {
		locations = append(locations, domain.FileManagerLocation{Type: domain.FileManagerLocationCloud115, ID: account.ID, Name: account.Name, Root: "0", Status: account.Status})
	}
	return locations, nil
}

// Browse 浏览指定位置的目录内容。
func (s *FileManagerService) Browse(location domain.FileManagerLocationRef, path string) (*domain.FileManagerBrowseResult, error) {
	switch location.Type {
	case domain.FileManagerLocationLocal:
		return s.browseLocal(location.ID, path)
	case domain.FileManagerLocationCloud115:
		return s.browseCloud115(location.ID, path)
	default:
		return nil, fmt.Errorf("不支持的位置类型: %s", location.Type)
	}
}

func (s *FileManagerService) browseLocal(sourceID int, path string) (*domain.FileManagerBrowseResult, error) {
	source, root, err := s.localSource(sourceID)
	if err != nil {
		return nil, err
	}
	fullPath, relPath, err := resolveLocalPath(root, path)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, fmt.Errorf("读取本地目录失败: %w", err)
	}
	result := &domain.FileManagerBrowseResult{Path: relPath, Entries: make([]domain.FileManagerEntry, 0, len(entries))}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		entryPath := filepath.ToSlash(filepath.Join(relPath, entry.Name()))
		result.Entries = append(result.Entries, domain.FileManagerEntry{ID: entryPath, Name: entry.Name(), Path: entryPath, ParentPath: relPath, IsDirectory: entry.IsDir(), Size: info.Size(), ModifiedAt: info.ModTime()})
	}
	sortFileManagerEntries(result.Entries)
	result.Total = len(result.Entries)
	logger.Debugf("FileManagerService[Browse] 本地位置=%s(%d), path=%s", source.Name, source.ID, relPath)
	return result, nil
}

func (s *FileManagerService) browseCloud115(accountID int, path string) (*domain.FileManagerBrowseResult, error) {
	account, err := s.cloudAccount(accountID)
	if err != nil {
		return nil, err
	}
	cid := normalizeCID(path)
	files, err := s.listAllCloudFiles(cid, account)
	if err != nil {
		return nil, err
	}
	result := &domain.FileManagerBrowseResult{Path: cid, Entries: make([]domain.FileManagerEntry, 0, len(files))}
	for _, file := range files {
		isDir := file.Type == "folder" || file.FileID == ""
		id := file.FileID
		if isDir {
			id = string(file.CategoryID)
		}
		result.Entries = append(result.Entries, domain.FileManagerEntry{ID: id, Name: file.Name, Path: id, ParentPath: cid, IsDirectory: isDir, Size: int64(file.Size), PickCode: file.PickCode, SHA1: file.Sha1})
	}
	sortFileManagerEntries(result.Entries)
	result.Total = len(result.Entries)
	return result, nil
}

// StartTransfer 创建复制或剪切粘贴任务并异步执行。
func (s *FileManagerService) StartTransfer(req domain.FileManagerTransferRequest) (*domain.FileManagerTransferResponse, error) {
	if req.Operation != domain.FileManagerOperationCopy && req.Operation != domain.FileManagerOperationMove {
		return nil, errors.New("operation仅支持copy或move")
	}
	if len(req.Items) == 0 {
		return nil, errors.New("至少选择一个文件或目录")
	}
	if _, err := s.resolveLocation(req.Source); err != nil {
		return nil, fmt.Errorf("源位置无效: %w", err)
	}
	if _, err := s.resolveLocation(req.Target); err != nil {
		return nil, fmt.Errorf("目标位置无效: %w", err)
	}

	taskID := uuid.NewString()
	taskName := fmt.Sprintf("文件%s - %d项", map[bool]string{true: "剪切", false: "复制"}[req.Operation == domain.FileManagerOperationMove], len(req.Items))
	if err := s.tasks.Create(taskID, fileManagerTaskType, taskName); err != nil {
		return nil, err
	}
	_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"operation": req.Operation, "source_type": req.Source.Type, "source_id": req.Source.ID, "target_type": req.Target.Type, "target_id": req.Target.ID, "target_path": req.TargetPath})

	ctx, cancel := context.WithCancel(context.Background())
	s.tasks.RegisterCancel(taskID, cancel)
	go s.runTransfer(ctx, taskID, req)
	return &domain.FileManagerTransferResponse{TaskID: taskID, Total: len(req.Items)}, nil
}

func (s *FileManagerService) runTransfer(ctx context.Context, taskID string, req domain.FileManagerTransferRequest) {
	defer s.tasks.RemoveCancel(taskID)
	_ = s.tasks.UpdateStatus(taskID, "running")
	total, success, failed := len(req.Items), 0, 0
	errorsFound := make([]string, 0)
	for index, item := range req.Items {
		if ctx.Err() != nil || s.tasks.IsCancelled(taskID) {
			return
		}
		if err := s.transferItem(ctx, req, item); err != nil {
			failed++
			errorsFound = append(errorsFound, fmt.Sprintf("%s: %v", item.Name, err))
			logger.Errorf("FileManagerService[Transfer] task=%s item=%s err=%v", taskID, item.Name, err)
		} else {
			success++
		}
		_ = s.tasks.UpdateProgress(taskID, total, index+1, success, failed)
	}
	if failed > 0 {
		_ = s.tasks.SetError(taskID, strings.Join(errorsFound, "; "))
		return
	}
	_ = s.tasks.UpdateStatus(taskID, "completed")
}

func (s *FileManagerService) transferItem(ctx context.Context, req domain.FileManagerTransferRequest, item domain.FileManagerTransferItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	move := req.Operation == domain.FileManagerOperationMove
	switch req.Source.Type + ":" + req.Target.Type {
	case domain.FileManagerLocationLocal + ":" + domain.FileManagerLocationLocal:
		return s.transferLocalToLocal(req.Source.ID, req.Target.ID, req.TargetPath, item, move)
	case domain.FileManagerLocationLocal + ":" + domain.FileManagerLocationCloud115:
		return s.transferLocalToCloud(ctx, req.Source.ID, req.Target.ID, req.TargetPath, item, move)
	case domain.FileManagerLocationCloud115 + ":" + domain.FileManagerLocationLocal:
		return s.transferCloudToLocal(ctx, req.Source.ID, req.Target.ID, req.TargetPath, item, move)
	case domain.FileManagerLocationCloud115 + ":" + domain.FileManagerLocationCloud115:
		return s.transferCloudToCloud(ctx, req.Source.ID, req.Target.ID, req.TargetPath, item, move)
	default:
		return errors.New("不支持的传输类型组合")
	}
}

func (s *FileManagerService) transferLocalToLocal(sourceID, targetID int, targetPath string, item domain.FileManagerTransferItem, move bool) error {
	_, sourceRoot, err := s.localSource(sourceID)
	if err != nil {
		return err
	}
	_, targetRoot, err := s.localSource(targetID)
	if err != nil {
		return err
	}
	sourcePath, _, err := resolveLocalPath(sourceRoot, item.Path)
	if err != nil {
		return err
	}
	targetDir, _, err := resolveLocalPath(targetRoot, targetPath)
	if err != nil {
		return err
	}
	targetFile := filepath.Join(targetDir, item.Name)
	if samePath(sourcePath, targetFile) {
		return errors.New("源路径和目标路径相同")
	}
	if err := copyLocalEntry(sourcePath, targetFile); err != nil {
		return err
	}
	if move {
		return os.RemoveAll(sourcePath)
	}
	return nil
}

func (s *FileManagerService) transferLocalToCloud(ctx context.Context, sourceID, targetID int, targetPath string, item domain.FileManagerTransferItem, move bool) error {
	_, sourceRoot, err := s.localSource(sourceID)
	if err != nil {
		return err
	}
	target, err := s.cloudAccount(targetID)
	if err != nil {
		return err
	}
	sourcePath, _, err := resolveLocalPath(sourceRoot, item.Path)
	if err != nil {
		return err
	}
	if err := s.uploadLocalEntry(ctx, sourcePath, normalizeCID(targetPath), target); err != nil {
		return err
	}
	if move {
		return os.RemoveAll(sourcePath)
	}
	return nil
}

func (s *FileManagerService) uploadLocalEntry(ctx context.Context, sourcePath, targetCID string, target *domain.Cloud115) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return s.cloudClient.UploadLocalFile115(sourcePath, targetCID, info.Name(), target.ID, target.Cookie)
	}
	dirCID, err := s.cloudClient.CreateDirectory115(targetCID, info.Name(), target.ID, target.Cookie)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(sourcePath)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := s.uploadLocalEntry(ctx, filepath.Join(sourcePath, entry.Name()), dirCID, target); err != nil {
			return err
		}
	}
	return nil
}

func (s *FileManagerService) transferCloudToLocal(ctx context.Context, sourceID, targetID int, targetPath string, item domain.FileManagerTransferItem, move bool) error {
	source, err := s.cloudAccount(sourceID)
	if err != nil {
		return err
	}
	_, targetRoot, err := s.localSource(targetID)
	if err != nil {
		return err
	}
	targetDir, _, err := resolveLocalPath(targetRoot, targetPath)
	if err != nil {
		return err
	}
	if err := s.downloadCloudItemToLocal(ctx, item, targetDir, source); err != nil {
		return err
	}
	if move {
		return s.cloudClient.DeleteFiles115([]string{item.ID}, source.ID, source.Cookie)
	}
	return nil
}

func (s *FileManagerService) downloadCloudItemToLocal(ctx context.Context, item domain.FileManagerTransferItem, targetDir string, source *domain.Cloud115) error {
	targetPath := filepath.Join(targetDir, item.Name)
	if !item.IsDirectory {
		return s.downloadCloudEntry(ctx, item, targetPath, source)
	}
	tempRoot, err := os.MkdirTemp(targetDir, ".easy-strm-download-dir-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempRoot)
	tempTarget := filepath.Join(tempRoot, item.Name)
	if err := s.downloadCloudEntry(ctx, item, tempTarget, source); err != nil {
		return err
	}
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return os.Rename(tempTarget, targetPath)
	} else if err != nil {
		return err
	}
	return copyLocalEntry(tempTarget, targetPath)
}

func (s *FileManagerService) downloadCloudEntry(ctx context.Context, item domain.FileManagerTransferItem, targetPath string, source *domain.Cloud115) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !item.IsDirectory {
		pickCode := item.PickCode
		if pickCode == "" {
			pickCode = item.ID
		}
		return s.cloudClient.DownloadFile115(pickCode, targetPath, source.ID, source.Cookie)
	}
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return err
	}
	children, err := s.listAllCloudFiles(item.ID, source)
	if err != nil {
		return err
	}
	for _, child := range cloudTransferItems(children) {
		if err := s.downloadCloudEntry(ctx, child, filepath.Join(targetPath, child.Name), source); err != nil {
			return err
		}
	}
	return nil
}

func (s *FileManagerService) transferCloudToCloud(ctx context.Context, sourceID, targetID int, targetPath string, item domain.FileManagerTransferItem, move bool) error {
	source, err := s.cloudAccount(sourceID)
	if err != nil {
		return err
	}
	target, err := s.cloudAccount(targetID)
	if err != nil {
		return err
	}
	targetCID := normalizeCID(targetPath)
	if sourceID == targetID {
		if move {
			return retryCloud115Operation(ctx, "同账号移动", func() error {
				return s.cloudClient.MoveFile115(item.ID, targetCID, source.ID, source.Cookie)
			})
		}
		return retryCloud115Operation(ctx, "同账号复制", func() error {
			return s.cloudClient.CopyFile(item.ID, targetCID, source.ID, source.Cookie)
		})
	}
	if err := s.copyCloudEntryAcrossAccounts(ctx, item, targetCID, source, target); err != nil {
		return err
	}
	if move {
		return retryCloud115Operation(ctx, "跨账号剪切删除源文件", func() error {
			return s.cloudClient.DeleteFiles115([]string{item.ID}, source.ID, source.Cookie)
		})
	}
	return nil
}

func (s *FileManagerService) copyCloudEntryAcrossAccounts(ctx context.Context, item domain.FileManagerTransferItem, targetCID string, source, target *domain.Cloud115) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !item.IsDirectory {
		pickCode := strings.TrimSpace(item.PickCode)
		if pickCode == "" {
			return errors.New("秒传失败：文件缺少秒传所需信息")
		}
		if strings.TrimSpace(item.SHA1) == "" || item.Size <= 0 {
			return errors.New("秒传失败：文件缺少秒传所需信息")
		}
		err := retryCloud115Operation(ctx, "跨账号秒传", func() error {
			_, transferErr := s.cloudClient.RapidTransferFileByMetadata(item.ID, pickCode, item.SHA1, item.Size, source.ID, source.Cookie, targetCID, target.ID, target.Cookie, item.Name)
			return transferErr
		})
		if err != nil {
			logger.Errorf("FileManagerService[RapidTransfer] source=%d target=%d file=%s raw_error=%v", source.ID, target.ID, item.Name, err)
			return errors.New(cloud115RapidTransferMessage(err))
		}
		return nil
	}
	dirCID, err := s.cloudClient.CreateDirectory115(targetCID, item.Name, target.ID, target.Cookie)
	if err != nil {
		return err
	}
	children, err := s.listAllCloudFiles(item.ID, source)
	if err != nil {
		return err
	}
	for _, child := range cloudTransferItems(children) {
		if err := s.copyCloudEntryAcrossAccounts(ctx, child, dirCID, source, target); err != nil {
			return err
		}
	}
	return nil
}

func retryCloud115Operation(ctx context.Context, operationName string, operation func() error) error {
	var lastErr error
	for attempt := 0; attempt <= cloud115OperationRetryCount; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := operation(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt == cloud115OperationRetryCount {
			break
		}
		delay := time.Duration(attempt+1) * cloud115RetryBaseDelay
		logger.Warnf("FileManagerService[Retry] operation=%s attempt=%d/%d error=%v", operationName, attempt+1, cloud115OperationRetryCount, lastErr)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func cloud115RapidTransferMessage(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "cookie"),
		strings.Contains(message, "credential"),
		strings.Contains(message, "unauthorized"),
		strings.Contains(message, "登录"),
		strings.Contains(message, "账号失效"):
		return "秒传失败：115账号登录状态已失效，请重新登录后重试"
	case strings.Contains(message, "sha1"),
		strings.Contains(message, "filesize"),
		strings.Contains(message, "file_id"),
		strings.Contains(message, "pickcode"),
		strings.Contains(message, "元数据"):
		return "秒传失败：文件缺少秒传所需信息"
	case strings.Contains(message, "need check"),
		strings.Contains(message, "content verification"),
		strings.Contains(message, "status=7"),
		strings.Contains(message, "签名校验"):
		return "秒传失败：115要求校验文件内容，当前文件暂不支持秒传"
	default:
		return "秒传失败：115暂未接受该文件，请稍后重试"
	}
}

// Delete 删除指定位置中的文件或目录。
func (s *FileManagerService) Delete(req domain.FileManagerDeleteRequest) error {
	if len(req.Items) == 0 {
		return errors.New("至少选择一个文件或目录")
	}
	switch req.Location.Type {
	case domain.FileManagerLocationLocal:
		_, root, err := s.localSource(req.Location.ID)
		if err != nil {
			return err
		}
		for _, item := range req.Items {
			fullPath, rel, resolveErr := resolveLocalPath(root, item.Path)
			if resolveErr != nil {
				return resolveErr
			}
			if rel == "" {
				return errors.New("不能删除媒体源根目录")
			}
			if err := os.RemoveAll(fullPath); err != nil {
				return fmt.Errorf("删除%s失败: %w", item.Name, err)
			}
		}
		return nil
	case domain.FileManagerLocationCloud115:
		account, err := s.cloudAccount(req.Location.ID)
		if err != nil {
			return err
		}
		ids := make([]string, 0, len(req.Items))
		for _, item := range req.Items {
			ids = append(ids, item.ID)
		}
		return s.cloudClient.DeleteFiles115(ids, account.ID, account.Cookie)
	default:
		return fmt.Errorf("不支持的位置类型: %s", req.Location.Type)
	}
}

func (s *FileManagerService) resolveLocation(ref domain.FileManagerLocationRef) (interface{}, error) {
	if ref.ID <= 0 {
		return nil, errors.New("位置ID无效")
	}
	if ref.Type == domain.FileManagerLocationLocal {
		source, _, err := s.localSource(ref.ID)
		return source, err
	}
	if ref.Type == domain.FileManagerLocationCloud115 {
		return s.cloudAccount(ref.ID)
	}
	return nil, fmt.Errorf("不支持的位置类型: %s", ref.Type)
}

func (s *FileManagerService) localSource(id int) (*domain.MediaSource, string, error) {
	source, err := s.mediaSources.GetByID(id)
	if err != nil {
		return nil, "", err
	}
	if source == nil || source.SourceType != domain.SourceTypeLocal {
		return nil, "", errors.New("本地媒体源不存在")
	}
	root, err := filepath.Abs(source.Path)
	if err != nil {
		return nil, "", err
	}
	return source, root, nil
}

func (s *FileManagerService) cloudAccount(id int) (*domain.Cloud115, error) {
	account, err := s.cloud115.GetByID(id)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, errors.New("115账号不存在")
	}
	return account, nil
}

func (s *FileManagerService) listAllCloudFiles(cid string, account *domain.Cloud115) ([]driver.FileInfo, error) {
	const pageSize = 1000
	result := make([]driver.FileInfo, 0)
	cidNumber, err := strconv.Atoi(normalizeCID(cid))
	if err != nil {
		return nil, fmt.Errorf("115目录ID无效: %s", cid)
	}
	for offset := 0; ; offset += pageSize {
		page, listErr := s.cloudClient.GetFileList(cidNumber, 1, offset, pageSize, account.ID, account.Cookie)
		if listErr != nil {
			return nil, fmt.Errorf("读取115目录失败: %w", listErr)
		}
		if page == nil || len(page.Files) == 0 {
			break
		}
		result = append(result, page.Files...)
		if len(page.Files) < pageSize {
			break
		}
	}
	return result, nil
}

func cloudTransferItems(files []driver.FileInfo) []domain.FileManagerTransferItem {
	items := make([]domain.FileManagerTransferItem, 0, len(files))
	for _, file := range files {
		isDir := file.Type == "folder" || file.FileID == ""
		id := file.FileID
		if isDir {
			id = string(file.CategoryID)
		}
		items = append(items, domain.FileManagerTransferItem{ID: id, Name: file.Name, Path: id, IsDirectory: isDir, PickCode: file.PickCode, SHA1: file.Sha1, Size: int64(file.Size)})
	}
	return items
}

func resolveLocalPath(root, relative string) (string, string, error) {
	relative = filepath.Clean(filepath.FromSlash(strings.TrimSpace(relative)))
	if relative == "." {
		relative = ""
	}
	fullPath, err := filepath.Abs(filepath.Join(root, relative))
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(root, fullPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", errors.New("路径超出媒体源根目录")
	}
	if rel == "." {
		rel = ""
	}
	return fullPath, filepath.ToSlash(rel), nil
}

func copyLocalEntry(source, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if samePath(source, target) || strings.HasPrefix(strings.ToLower(filepath.Clean(target))+string(filepath.Separator), strings.ToLower(filepath.Clean(source))+string(filepath.Separator)) {
			return errors.New("不能将目录复制到自身或其子目录")
		}
		if err := os.MkdirAll(target, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyLocalEntry(filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func samePath(left, right string) bool {
	leftAbs, _ := filepath.Abs(left)
	rightAbs, _ := filepath.Abs(right)
	return strings.EqualFold(filepath.Clean(leftAbs), filepath.Clean(rightAbs))
}

func normalizeCID(cid string) string {
	cid = strings.TrimSpace(cid)
	if cid == "" || cid == "/" {
		return "0"
	}
	return cid
}

func sortFileManagerEntries(entries []domain.FileManagerEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDirectory != entries[j].IsDirectory {
			return entries[i].IsDirectory
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
}
