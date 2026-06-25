package file

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuditFileUploadSkipsWhenAuditRepositoryMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	h := &Handler{}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("auditFileUpload panicked when audit repository was missing: %v", r)
		}
	}()

	h.auditFileUpload(c, &File{
		ID:               "file-1",
		BusinessAppCode:  "finance",
		StorageKey:       "file-1.csv",
		OriginalFilename: "source.csv",
		SizeBytes:        123,
	})
}
