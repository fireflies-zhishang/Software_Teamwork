package service

import (
	"io"
	"time"
)

type DocumentStatus string

const (
	DocumentStatusUploaded DocumentStatus = "uploaded"
)

type RequestContext struct {
	RequestID      string
	UserID         string
	CallerService  string
	ServiceToken   string
	Roles          []string
	Permissions    []string
	ForwardedFor   string
	ForwardedProto string
}

type Document struct {
	ID              string
	KnowledgeBaseID string
	Name            string
	Status          DocumentStatus
	Tags            []string
	ErrorMessage    *string
	CreatedAt       time.Time
	ContentType     string
	SizeBytes       int64
	ObjectKey       string
	OwnerUserID     string
	DeletedAt       *time.Time
}

type StoredObject struct {
	Body        io.ReadCloser
	ContentType string
	SizeBytes   int64
}

type FileStatus string

const (
	FileStatusAvailable       FileStatus = "available"
	FileStatusDeleteRequested FileStatus = "delete_requested"
	FileStatusPurging         FileStatus = "purging"
	FileStatusPurged          FileStatus = "purged"
	FileStatusFailed          FileStatus = "failed"
)

type FileObject struct {
	ID                string
	Filename          string
	ContentType       string
	SizeBytes         int64
	ChecksumSHA256    string
	StorageBackend    string
	StorageBucket     string
	StorageObjectKey  string
	StorageVersionID  string
	StorageETag       string
	Status            FileStatus
	CreatedByService  string
	RequestID         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         *time.Time
	DeleteRequestedAt *time.Time
	PurgedAt          *time.Time
	LastErrorCode     string
	LastErrorMessage  string
}

type FileContent struct {
	File        FileObject
	Body        io.ReadCloser
	ContentType string
	SizeBytes   int64
}

type DocumentContent struct {
	Document    Document
	Body        io.ReadCloser
	ContentType string
	SizeBytes   int64
}

type UploadDocumentInput struct {
	KnowledgeBaseID string
	FileName        string
	ContentType     string
	SizeBytes       int64
	Tags            []string
	Content         io.Reader
}

type UpdateDocumentInput struct {
	DocumentID string
	Tags       []string
}

type CreateFileInput struct {
	FileName       string
	ContentType    string
	SizeBytes      int64
	ChecksumSHA256 string
	Content        io.Reader
}
