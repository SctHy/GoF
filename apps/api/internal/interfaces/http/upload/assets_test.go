package interfaceshttpupload

import (
	middleware "GCFeed/internal/interfaces/http/middleware"
	"bytes"
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type registrar struct {
	owner int64
	size  int64
	err   error
}

func (r *registrar) Register(_ context.Context, owner int64, _, _ string, size int64) error {
	r.owner = owner
	r.size = size
	return r.err
}

func TestUploadRegistrationAndFailureCleanup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, fail := range []bool{false, true} {
		root := t.TempDir()
		store := &registrar{}
		if fail {
			store.err = errors.New("database unavailable")
		}
		router := gin.New()
		router.POST("/upload", func(c *gin.Context) { c.Set(middleware.ContextUserIDKey, int64(42)) }, New(root, store).Create)
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		_ = form.WriteField("kind", "cover")
		part, _ := form.CreateFormFile("file", "cover.png")
		// PNG magic accepted by the existing MIME validator.
		_, _ = part.Write([]byte("\x89PNG\r\n\x1a\n"))
		_ = form.Close()
		req := httptest.NewRequest("POST", "/upload", &body)
		req.Header.Set("Content-Type", form.FormDataContentType())
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		expected := 201
		if fail {
			expected = 500
		}
		if resp.Code != expected || store.owner != 42 || store.size != 8 {
			t.Fatalf("status=%d registration=%+v", resp.Code, store)
		}
		entries, err := os.ReadDir(filepath.Join(root, "cover"))
		if err != nil {
			t.Fatal(err)
		}
		if (fail && len(entries) != 0) || (!fail && len(entries) != 1) {
			t.Fatalf("files after upload: %d", len(entries))
		}
	}
}
