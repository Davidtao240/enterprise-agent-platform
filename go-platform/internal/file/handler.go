package file

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/enterprise-agent-platform/go-platform/internal/audit"
	"github.com/enterprise-agent-platform/go-platform/internal/platform"
	"github.com/enterprise-agent-platform/go-platform/pkg/apierror"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	repo       *Repository
	audit      *audit.Repository
	bucket     string
	storageDir string
}

func NewHandler(repo *Repository, auditRepo *audit.Repository, bucket, storageDir string) *Handler {
	return &Handler{repo: repo, audit: auditRepo, bucket: bucket, storageDir: storageDir}
}

func (h *Handler) Upload(c *gin.Context) {
	businessAppCode := c.PostForm("business_app_code")
	if businessAppCode == "" {
		businessAppCode = "finance"
	}
	fileRole := c.PostForm("file_role")
	if fileRole == "" {
		fileRole = "source"
	}

	header, err := c.FormFile("file")
	if err != nil {
		platform.APIError(c, apierror.ErrValidationFailed)
		return
	}
	src, err := header.Open()
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	defer src.Close()

	id := uuid.New().String()
	ext := strings.ToLower(filepath.Ext(header.Filename))
	storageKey := id + ext
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	if err := os.MkdirAll(h.storageDir, 0o755); err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	dstPath := filepath.Join(h.storageDir, storageKey)
	dst, err := os.Create(dstPath)
	if err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	defer dst.Close()

	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(dst, hash), src); err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}
	checksum := hex.EncodeToString(hash.Sum(nil))

	userID := c.GetString("user_id")
	var uploadedBy *string
	if userID != "" {
		uploadedBy = &userID
	}
	workflowID := c.PostForm("workflow_instance_id")
	var workflowIDPtr *string
	if workflowID != "" {
		workflowIDPtr = &workflowID
	}

	record := &File{
		ID:                 id,
		WorkflowInstanceID: workflowIDPtr,
		BusinessAppCode:    businessAppCode,
		StorageBucket:      h.bucket,
		StorageKey:         storageKey,
		OriginalFilename:   header.Filename,
		ContentType:        contentType,
		SizeBytes:          header.Size,
		FileRole:           fileRole,
		UploadedBy:         uploadedBy,
		Checksum:           &checksum,
	}
	if err := h.repo.Create(c.Request.Context(), record); err != nil {
		platform.APIError(c, apierror.ErrInternalError)
		return
	}

	h.auditFileUpload(c, record)
	platform.Success(c, gin.H{
		"id":                   record.ID,
		"file_id":              record.StorageKey,
		"storage_key":          record.StorageKey,
		"original_filename":    record.OriginalFilename,
		"business_app_code":    record.BusinessAppCode,
		"workflow_instance_id": record.WorkflowInstanceID,
		"file_role":            record.FileRole,
		"size_bytes":           record.SizeBytes,
	})
}

func (h *Handler) Get(c *gin.Context) {
	f, err := h.repo.FindByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		platform.APIError(c, apierror.ErrResourceNotFound)
		return
	}
	platform.Success(c, f)
}

func (h *Handler) GetContent(c *gin.Context) {
	storageKey := filepath.Base(c.Param("storage_key"))
	if storageKey == "." || storageKey == string(filepath.Separator) || storageKey == "" {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	c.File(filepath.Join(h.storageDir, storageKey))
}

func (h *Handler) auditFileUpload(c *gin.Context, f *File) {
	if h.audit == nil {
		return
	}
	detail := fmt.Sprintf(`{"file_id":%q,"original_filename":%q,"size_bytes":%d}`, f.StorageKey, f.OriginalFilename, f.SizeBytes)
	userID := c.GetString("user_id")
	var actor *string
	if userID != "" {
		actor = &userID
	}
	_, _, _ = h.audit.InsertLog(c.Request.Context(), audit.AuditLogEntry{
		TraceID:         c.GetHeader("X-Trace-Id"),
		ActorUserID:     actor,
		BusinessAppCode: &f.BusinessAppCode,
		Action:          "file_uploaded",
		ResourceType:    "file",
		ResourceID:      f.ID,
		Status:          "succeeded",
		DetailJSON:      &detail,
	})
}
