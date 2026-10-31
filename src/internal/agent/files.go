package agent

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	agentv1 "dockium/gen/agent/v1"
)

const (
	maxReadSize   = 2 << 20
	maxWriteSize  = 4 << 20
	maxUploadSize = 32 << 20
	maxEntries    = 5000
)

var (
	errInvalidPath = errors.New("некорректный путь")
	errNotFound    = errors.New("файл или каталог не найден")
	errExists      = errors.New("файл или каталог уже существует")
	errIsDirectory = errors.New("это каталог")
	errNotDir      = errors.New("это не каталог")
	errTooLarge    = errors.New("файл слишком большой")
)

type fileService struct {
	manager *Manager
}

func newFileService(manager *Manager) *fileService {
	return &fileService{manager: manager}
}

func (f *fileService) root(instanceID int64) (string, error) {
	if instanceID <= 0 {
		return "", errInvalidPath
	}
	root := f.manager.InstanceDirectory(instanceID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("не удалось открыть каталог данных: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		return resolved, nil
	}
	return root, nil
}

func cleanRelative(value string) (string, error) {
	if strings.ContainsRune(value, 0) {
		return "", errInvalidPath
	}
	normalized := strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	normalized = strings.TrimPrefix(normalized, "/")
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return "", errInvalidPath
		}
	}
	cleaned := path.Clean("/" + normalized)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "." {
		cleaned = ""
	}
	return cleaned, nil
}

func within(base, target string) bool {
	relative, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func evalExistingAncestor(target string) (string, bool) {
	current := target
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			return resolved, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

func (f *fileService) resolve(instanceID int64, value string, mustExist bool) (string, string, error) {
	root, err := f.root(instanceID)
	if err != nil {
		return "", "", err
	}
	relative, err := cleanRelative(value)
	if err != nil {
		return "", "", err
	}
	full := filepath.Join(root, filepath.FromSlash(relative))
	if !within(root, full) {
		return "", "", errInvalidPath
	}
	resolved, ok := evalExistingAncestor(full)
	if !ok {
		return "", "", errInvalidPath
	}
	if !within(root, resolved) {
		return "", "", errInvalidPath
	}
	if mustExist {
		if _, err := os.Lstat(full); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return "", "", errNotFound
			}
			return "", "", fmt.Errorf("проверить путь: %w", err)
		}
	}
	return full, relative, nil
}

func (f *fileService) handleList(id int64, request *agentv1.ListFiles) *agentv1.CommandResult {
	full, relative, err := f.resolve(request.InstanceId, request.Path, true)
	if err != nil {
		return result(id, nil, err)
	}
	info, err := os.Stat(full)
	if err != nil {
		return result(id, nil, errNotFound)
	}
	if !info.IsDir() {
		return result(id, nil, errNotDir)
	}
	items, err := os.ReadDir(full)
	if err != nil {
		return result(id, nil, fmt.Errorf("прочитать каталог: %w", err))
	}
	entries := make([]*agentv1.FileEntry, 0, len(items))
	for _, item := range items {
		if len(entries) >= maxEntries {
			break
		}
		childPath := item.Name()
		if relative != "" {
			childPath = relative + "/" + item.Name()
		}
		info, err := item.Info()
		if err != nil {
			continue
		}
		entries = append(entries, &agentv1.FileEntry{
			Name:       item.Name(),
			Path:       childPath,
			IsDir:      info.IsDir(),
			Size:       uint64(max(info.Size(), 0)),
			ModifiedAt: info.ModTime().UnixMilli(),
			Mode:       uint32(info.Mode().Perm()),
		})
	}
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].IsDir != entries[right].IsDir {
			return entries[left].IsDir
		}
		return strings.ToLower(entries[left].Name) < strings.ToLower(entries[right].Name)
	})
	return result(id, &agentv1.CommandResult_Files{Files: &agentv1.FileList{Items: entries, Path: relative}}, nil)
}

func (f *fileService) handleRead(id int64, request *agentv1.ReadFile) *agentv1.CommandResult {
	full, _, err := f.resolve(request.InstanceId, request.Path, true)
	if err != nil {
		return result(id, nil, err)
	}
	file, err := os.Open(full)
	if err != nil {
		return result(id, nil, errNotFound)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return result(id, nil, fmt.Errorf("прочитать файл: %w", err))
	}
	if info.IsDir() {
		return result(id, nil, errIsDirectory)
	}
	if info.Size() > maxReadSize {
		return result(id, &agentv1.CommandResult_File{File: &agentv1.FileContent{
			Size:       uint64(info.Size()),
			ModifiedAt: info.ModTime().UnixMilli(),
			Binary:     true,
		}}, nil)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxReadSize+1))
	if err != nil {
		return result(id, nil, fmt.Errorf("прочитать файл: %w", err))
	}
	binary := !utf8.Valid(data) || strings.ContainsRune(string(data), 0)
	content := &agentv1.FileContent{
		Size:       uint64(info.Size()),
		ModifiedAt: info.ModTime().UnixMilli(),
		Binary:     binary,
	}
	if !binary {
		content.Content = data
	}
	return result(id, &agentv1.CommandResult_File{File: content}, nil)
}

func (f *fileService) handleWrite(id int64, request *agentv1.WriteFile) *agentv1.CommandResult {
	if len(request.Content) > maxWriteSize {
		return result(id, nil, errTooLarge)
	}
	full, _, err := f.resolve(request.InstanceId, request.Path, false)
	if err != nil {
		return result(id, nil, err)
	}
	info, statErr := os.Stat(full)
	if statErr == nil && info.IsDir() {
		return result(id, nil, errIsDirectory)
	}
	if request.Create && statErr == nil {
		return result(id, nil, errExists)
	}
	if !request.Create && errors.Is(statErr, fs.ErrNotExist) {
		return result(id, nil, errNotFound)
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return result(id, nil, fmt.Errorf("создать каталог: %w", err))
	}
	mode := os.FileMode(0o644)
	if statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := writeAtomic(full, request.Content, mode); err != nil {
		return result(id, nil, err)
	}
	return result(id, &agentv1.CommandResult_Empty{Empty: &agentv1.Empty{}}, nil)
}

func (f *fileService) handleMkdir(id int64, request *agentv1.CreateDirectory) *agentv1.CommandResult {
	full, _, err := f.resolve(request.InstanceId, request.Path, false)
	if err != nil {
		return result(id, nil, err)
	}
	if _, err := os.Lstat(full); err == nil {
		return result(id, nil, errExists)
	}
	if err := os.MkdirAll(full, 0o755); err != nil {
		return result(id, nil, fmt.Errorf("создать каталог: %w", err))
	}
	return result(id, &agentv1.CommandResult_Empty{Empty: &agentv1.Empty{}}, nil)
}

func (f *fileService) handleDelete(id int64, request *agentv1.DeletePath) *agentv1.CommandResult {
	full, relative, err := f.resolve(request.InstanceId, request.Path, true)
	if err != nil {
		return result(id, nil, err)
	}
	if relative == "" {
		return result(id, nil, errors.New("нельзя удалить корень данных экземпляра"))
	}
	info, err := os.Lstat(full)
	if err != nil {
		return result(id, nil, errNotFound)
	}
	if info.IsDir() && !request.Recursive {
		items, err := os.ReadDir(full)
		if err != nil {
			return result(id, nil, fmt.Errorf("прочитать каталог: %w", err))
		}
		if len(items) > 0 {
			return result(id, nil, errors.New("каталог не пуст"))
		}
	}
	var removeErr error
	if info.IsDir() && request.Recursive {
		removeErr = os.RemoveAll(full)
	} else {
		removeErr = os.Remove(full)
	}
	if removeErr != nil {
		return result(id, nil, fmt.Errorf("удалить: %w", removeErr))
	}
	return result(id, &agentv1.CommandResult_Empty{Empty: &agentv1.Empty{}}, nil)
}

func (f *fileService) handleRename(id int64, request *agentv1.RenamePath) *agentv1.CommandResult {
	source, relative, err := f.resolve(request.InstanceId, request.OldPath, true)
	if err != nil {
		return result(id, nil, err)
	}
	if relative == "" {
		return result(id, nil, errInvalidPath)
	}
	target, _, err := f.resolve(request.InstanceId, request.NewPath, false)
	if err != nil {
		return result(id, nil, err)
	}
	if _, err := os.Lstat(target); err == nil {
		return result(id, nil, errExists)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return result(id, nil, fmt.Errorf("создать каталог: %w", err))
	}
	if err := os.Rename(source, target); err != nil {
		return result(id, nil, fmt.Errorf("переименовать: %w", err))
	}
	return result(id, &agentv1.CommandResult_Empty{Empty: &agentv1.Empty{}}, nil)
}

func (f *fileService) handleUpload(id int64, request *agentv1.UploadFile) *agentv1.CommandResult {
	if len(request.Content) > maxUploadSize {
		return result(id, nil, errTooLarge)
	}
	full, _, err := f.resolve(request.InstanceId, request.Path, false)
	if err != nil {
		return result(id, nil, err)
	}
	if _, err := os.Stat(full); err == nil && !request.Overwrite {
		return result(id, nil, errExists)
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return result(id, nil, fmt.Errorf("создать каталог: %w", err))
	}
	if err := writeAtomic(full, request.Content, 0o644); err != nil {
		return result(id, nil, err)
	}
	return result(id, &agentv1.CommandResult_Empty{Empty: &agentv1.Empty{}}, nil)
}

func (f *fileService) handleChmod(id int64, request *agentv1.ChmodPath) *agentv1.CommandResult {
	full, _, err := f.resolve(request.InstanceId, request.Path, true)
	if err != nil {
		return result(id, nil, err)
	}
	if request.Mode > 0o777 {
		return result(id, nil, errors.New("некорректные права доступа"))
	}
	if err := os.Chmod(full, os.FileMode(request.Mode)); err != nil {
		return result(id, nil, fmt.Errorf("изменить права: %w", err))
	}
	return result(id, &agentv1.CommandResult_Empty{Empty: &agentv1.Empty{}}, nil)
}

func (f *fileService) handleCompress(id int64, request *agentv1.CompressPath) *agentv1.CommandResult {
	source, relative, err := f.resolve(request.InstanceId, request.Path, true)
	if err != nil {
		return result(id, nil, err)
	}
	if relative == "" {
		return result(id, nil, errInvalidPath)
	}
	targetName := request.Target
	if targetName == "" {
		targetName = relative + ".zip"
	}
	target, targetRelative, err := f.resolve(request.InstanceId, targetName, false)
	if err != nil {
		return result(id, nil, err)
	}
	if _, err := os.Stat(target); err == nil && !request.Overwrite {
		return result(id, nil, errExists)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return result(id, nil, fmt.Errorf("создать каталог: %w", err))
	}
	if err := compress(source, target); err != nil {
		return result(id, nil, err)
	}
	return result(id, &agentv1.CommandResult_Path{Path: &agentv1.PathInfo{Path: targetRelative}}, nil)
}

func (f *fileService) handleDownload(id int64, request *agentv1.DownloadFile) *agentv1.CommandResult {
	full, _, err := f.resolve(request.InstanceId, request.Path, true)
	if err != nil {
		f.sendFileError(request.StreamId, err)
		return result(id, nil, err)
	}
	info, err := os.Stat(full)
	if err != nil {
		f.sendFileError(request.StreamId, errNotFound)
		return result(id, nil, errNotFound)
	}
	if info.IsDir() {
		f.sendFileError(request.StreamId, errIsDirectory)
		return result(id, nil, errIsDirectory)
	}
	go f.streamFile(request.StreamId, full)
	return result(id, &agentv1.CommandResult_Empty{Empty: &agentv1.Empty{}}, nil)
}

func (f *fileService) streamFile(streamID, full string) {
	file, err := os.Open(full)
	if err != nil {
		f.sendFileError(streamID, err)
		return
	}
	defer file.Close()
	buffer := make([]byte, 64*1024)
	for {
		read, err := file.Read(buffer)
		if read > 0 {
			chunk := make([]byte, read)
			copy(chunk, buffer[:read])
			f.manager.send(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_FileChunk{FileChunk: &agentv1.FileChunk{
				StreamId: streamID,
				Data:     chunk,
			}}})
		}
		if err == io.EOF {
			f.manager.send(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_FileChunk{FileChunk: &agentv1.FileChunk{
				StreamId: streamID,
				Eof:      true,
			}}})
			return
		}
		if err != nil {
			f.sendFileError(streamID, err)
			return
		}
	}
}

func (f *fileService) sendFileError(streamID string, err error) {
	f.manager.send(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_FileChunk{FileChunk: &agentv1.FileChunk{
		StreamId: streamID,
		Eof:      true,
		Error:    err.Error(),
	}}})
}

func writeAtomic(target string, data []byte, mode os.FileMode) error {
	directory := filepath.Dir(target)
	temp, err := os.CreateTemp(directory, ".dockium-*")
	if err != nil {
		return fmt.Errorf("создать временный файл: %w", err)
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("записать файл: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("записать файл: %w", err)
	}
	if err := os.Chmod(name, mode); err != nil {
		return fmt.Errorf("изменить права: %w", err)
	}
	if err := os.Rename(name, target); err != nil {
		_ = os.Remove(target)
		if retry := os.Rename(name, target); retry != nil {
			return fmt.Errorf("записать файл: %w", err)
		}
	}
	return nil
}

func compress(source, target string) error {
	temp, err := os.CreateTemp(filepath.Dir(target), ".dockium-zip-*")
	if err != nil {
		return fmt.Errorf("создать архив: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	writer := zip.NewWriter(temp)
	rootParent := filepath.Dir(source)
	walkErr := filepath.WalkDir(source, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(rootParent, current)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() && current == source {
			return nil
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = relative
		if info.IsDir() {
			header.Name += "/"
			_, err = writer.CreateHeader(header)
			return err
		}
		header.Method = zip.Deflate
		entryWriter, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		file, err := os.Open(current)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(entryWriter, file)
		return err
	})
	if walkErr != nil {
		writer.Close()
		temp.Close()
		return fmt.Errorf("создать архив: %w", walkErr)
	}
	if err := writer.Close(); err != nil {
		temp.Close()
		return fmt.Errorf("создать архив: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("создать архив: %w", err)
	}
	if err := os.Rename(tempName, target); err != nil {
		return fmt.Errorf("сохранить архив: %w", err)
	}
	return nil
}
