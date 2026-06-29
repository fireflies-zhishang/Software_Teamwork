package config_test

import (
	"testing"

	"github.com/Sakayori-Iroha-168/Software_Teamwork/services/file/internal/config"
)

func TestLoadAcceptsLocalStorageBackend(t *testing.T) {
	t.Setenv("FILE_STORAGE_BACKEND", "local")
	t.Setenv("FILE_LOCAL_STORAGE_DIR", t.TempDir())

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.StorageBackend != "local" || cfg.LocalStorageDir == "" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestLoadRejectsUnsupportedStorageBackend(t *testing.T) {
	t.Setenv("FILE_STORAGE_BACKEND", "minio")

	if _, err := config.Load(); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}
