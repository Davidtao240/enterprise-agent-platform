package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

type PythonEmbeddingClient struct {
	baseURL string
	client  *http.Client
}

func NewPythonEmbeddingClient(baseURL string) *PythonEmbeddingClient {
	return &PythonEmbeddingClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *PythonEmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]any{"text": text})
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("embedding service returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}
	return result.Embedding, nil
}

type Service struct {
	repo      *Repository
	embClient *PythonEmbeddingClient
	uploadDir string
}

func NewService(repo *Repository, agentServiceURL, uploadDir string) *Service {
	return &Service{
		repo:      repo,
		embClient: NewPythonEmbeddingClient(agentServiceURL),
		uploadDir: uploadDir,
	}
}

// ── Collection methods ──────────────────────────────────────────

func (s *Service) CreateCollection(ctx context.Context, tenantID, userID, name, description string) (*Collection, error) {
	c := &Collection{
		ID:             uuid.NewString(),
		TenantID:       tenantID,
		Name:           name,
		Description:    description,
		EmbeddingModel: "text-embedding-3-small",
		ChunkSize:      1000,
		ChunkOverlap:   100,
		Status:         "active",
		CreatedBy:      userID,
	}
	if err := s.repo.CreateCollection(ctx, c); err != nil {
		return nil, fmt.Errorf("create collection: %w", err)
	}
	return c, nil
}

func (s *Service) ListCollections(ctx context.Context, tenantID string) ([]Collection, error) {
	return s.repo.ListCollections(ctx, tenantID)
}

func (s *Service) GetCollection(ctx context.Context, id, tenantID string) (*Collection, error) {
	return s.repo.GetCollection(ctx, id, tenantID)
}

func (s *Service) DeleteCollection(ctx context.Context, id, tenantID string) error {
	return s.repo.DeleteCollection(ctx, id, tenantID)
}

// ── Document methods ──────────────────────────────────────────

func (s *Service) UploadDocument(ctx context.Context, tenantID, userID, collectionID string,
	file io.Reader, fileName string) (*Document, error) {

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	mimeType := detectMimeType(fileName, content)
	ext := filepath.Ext(fileName)
	storagePath := filepath.Join(s.uploadDir, tenantID, uuid.NewString()+ext)

	os.MkdirAll(filepath.Dir(storagePath), 0o755)
	if err := os.WriteFile(storagePath, content, 0o644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	doc := &Document{
		ID:           uuid.NewString(),
		TenantID:     tenantID,
		CollectionID: collectionID,
		FileName:     fileName,
		FilePath:     storagePath,
		FileSize:     int64(len(content)),
		MimeType:     mimeType,
		Status:       "processing",
		CreatedBy:    userID,
	}
	if err := s.repo.CreateDocument(ctx, doc); err != nil {
		return nil, fmt.Errorf("create document: %w", err)
	}

	go s.processDocument(context.Background(), doc, string(content))
	return doc, nil
}

func (s *Service) processDocument(ctx context.Context, doc *Document, content string) {
	if len(content) == 0 {
		content = "(empty document)"
	}

	chunks := chunkText(content, 1000, 100)

	var chunkRecords []Chunk
	for i, text := range chunks {
		chunkRecords = append(chunkRecords, Chunk{
			ID:         uuid.NewString(),
			DocumentID: doc.ID,
			TenantID:   doc.TenantID,
			ChunkIndex: i,
			Content:    text,
			Metadata:   map[string]any{"document_id": doc.ID, "chunk_index": i},
		})
	}

	if err := s.repo.InsertChunks(ctx, chunkRecords, "text-embedding-3-small"); err != nil {
		doc.Status = "failed"
		doc.ErrorMsg = err.Error()
		doc.ChunkCount = 0
		_ = s.repo.UpdateDocument(ctx, doc)
		return
	}

	chunkIDs := make([]string, len(chunkRecords))
	for i, c := range chunkRecords {
		chunkIDs[i] = c.ID
	}

	var embeddings [][]float32
	for _, c := range chunkRecords {
		emb, err := s.embClient.Embed(ctx, c.Content)
		if err != nil {
			continue
		}
		embeddings = append(embeddings, emb)
	}

	if err := s.repo.UpdateChunkEmbeddings(ctx, chunkIDs[:len(embeddings)], embeddings); err != nil {
		doc.Status = "failed"
		doc.ErrorMsg = err.Error()
		doc.ChunkCount = len(chunkRecords)
		_ = s.repo.UpdateDocument(ctx, doc)
		return
	}

	now := time.Now()
	doc.Status = "ready"
	doc.ChunkCount = len(chunkRecords)
	doc.ProcessedAt = now
	doc.ErrorMsg = ""
	_ = s.repo.UpdateDocument(ctx, doc)
}

func (s *Service) ListDocuments(ctx context.Context, tenantID, collectionID, status string) ([]Document, error) {
	return s.repo.ListDocuments(ctx, tenantID, collectionID, status)
}

func (s *Service) DeleteDocument(ctx context.Context, id, tenantID string) error {
	return s.repo.DeleteDocument(ctx, id, tenantID)
}

// ── Search ──────────────────────────────────────────

func (s *Service) Search(ctx context.Context, tenantID, collectionID, query string, topK int, minScore float64) ([]SearchResult, error) {
	if topK <= 0 {
		topK = 10
	}

	embedding, err := s.embClient.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	return s.repo.SearchByVector(ctx, tenantID, collectionID, embedding, topK, minScore)
}

// ── Helpers ──────────────────────────────────────────

func detectMimeType(fileName string, content []byte) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".txt":
		return "text/plain"
	case ".md":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".csv":
		return "text/csv"
	case ".html":
		return "text/html"
	case ".pdf":
		return "application/pdf"
	case ".docx", ".doc":
		return "application/msword"
	default:
		return http.DetectContentType(content)
	}
}

func chunkText(text string, chunkSize, overlap int) []string {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}

	var chunks []string
	step := chunkSize - overlap

	for start := 0; start < len(runes); start += step {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunk := string(runes[start:end])
		chunk = strings.TrimSpace(chunk)
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end >= len(runes) {
			break
		}
	}
	return chunks
}