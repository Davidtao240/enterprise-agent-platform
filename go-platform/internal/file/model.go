package file

import "time"

type File struct {
	ID                 string    `json:"id"`
	WorkflowInstanceID *string   `json:"workflow_instance_id,omitempty"`
	BusinessAppCode    string    `json:"business_app_code"`
	StorageBucket      string    `json:"storage_bucket"`
	StorageKey         string    `json:"storage_key"`
	OriginalFilename   string    `json:"original_filename"`
	ContentType        string    `json:"content_type"`
	SizeBytes          int64     `json:"size_bytes"`
	FileRole           string    `json:"file_role"`
	UploadedBy         *string   `json:"uploaded_by,omitempty"`
	Checksum           *string   `json:"checksum,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}
