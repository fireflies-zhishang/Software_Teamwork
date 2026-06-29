package storage_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/file/internal/platform/storage"
	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/file/internal/service"
)

func TestLocalStorePutGetDelete(t *testing.T) {
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalStore() error = %v", err)
	}
	if err := store.Put(context.Background(), "files/file_1", strings.NewReader("content"), "text/plain", int64(len("content"))); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	object, err := store.Get(context.Background(), "files/file_1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	body, err := io.ReadAll(object.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if err := object.Body.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if string(body) != "content" || object.ContentType != "text/plain" || object.SizeBytes != int64(len("content")) {
		t.Fatalf("object = %+v, body = %q", object, string(body))
	}

	if err := store.Delete(context.Background(), "files/file_1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Get(context.Background(), "files/file_1"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestLocalStoreRejectsPathTraversal(t *testing.T) {
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalStore() error = %v", err)
	}
	if err := store.Put(context.Background(), "../outside", strings.NewReader("content"), "text/plain", int64(len("content"))); err == nil {
		t.Fatal("Put() error = nil, want error")
	}
}
