package storage

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/file/internal/service"
)

type LocalStore struct {
	root string
}

type localMetadata struct {
	ContentType string `json:"contentType"`
	SizeBytes   int64  `json:"sizeBytes"`
}

func NewLocalStore(root string) (*LocalStore, error) {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return nil, errors.New("local storage root is required")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &LocalStore{root: abs}, nil
}

func (s *LocalStore) Put(ctx context.Context, key string, body io.Reader, contentType string, sizeBytes int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if body == nil {
		return service.ErrNotFound
	}
	path, err := s.objectPath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".upload-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	written, copyErr := io.Copy(tmp, body)
	closeErr := tmp.Close()
	if copyErr != nil {
		_ = os.Remove(tmpName)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpName)
		return closeErr
	}
	if sizeBytes >= 0 && written != sizeBytes {
		_ = os.Remove(tmpName)
		return service.ErrConflict
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}

	metadata := localMetadata{ContentType: contentType, SizeBytes: written}
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return os.WriteFile(metadataPath(path), metadataBytes, 0o644)
}

func (s *LocalStore) Get(ctx context.Context, key string) (service.StoredObject, error) {
	if err := ctx.Err(); err != nil {
		return service.StoredObject{}, err
	}
	path, err := s.objectPath(key)
	if err != nil {
		return service.StoredObject{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return service.StoredObject{}, service.ErrNotFound
		}
		return service.StoredObject{}, err
	}

	metadata := localMetadata{ContentType: "application/octet-stream", SizeBytes: -1}
	if metadataBytes, err := os.ReadFile(metadataPath(path)); err == nil {
		_ = json.Unmarshal(metadataBytes, &metadata)
	}
	if metadata.SizeBytes < 0 {
		if stat, err := file.Stat(); err == nil {
			metadata.SizeBytes = stat.Size()
		}
	}
	return service.StoredObject{Body: file, ContentType: metadata.ContentType, SizeBytes: metadata.SizeBytes}, nil
}

func (s *LocalStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.objectPath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return service.ErrNotFound
		}
		return err
	}
	if err := os.Remove(metadataPath(path)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *LocalStore) objectPath(key string) (string, error) {
	cleanKey := filepath.Clean(strings.ReplaceAll(strings.TrimSpace(key), "\\", "/"))
	if cleanKey == "." || cleanKey == string(filepath.Separator) || strings.HasPrefix(cleanKey, "..") || filepath.IsAbs(cleanKey) {
		return "", errors.New("invalid storage object key")
	}
	path := filepath.Join(s.root, cleanKey)
	if !strings.HasPrefix(path, s.root+string(filepath.Separator)) && path != s.root {
		return "", errors.New("invalid storage object key")
	}
	return path, nil
}

func metadataPath(path string) string {
	return path + ".meta.json"
}
