package service

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"github.com/golllm/cmd/rag/model"
	"github.com/golllm/cmd/rag/repository"
	"github.com/golllm/pkg/embedding"
	"github.com/golllm/pkg/text"
	"github.com/golllm/pkg/vector"
)

// DocumentService 文档服务
type DocumentService struct {
	ctx       context.Context
	embedder  *embedding.Client
	vectorDB  vector.VectorClient
	splitter  text.Splitter
	repo      *repository.SQLiteRepository
	uploadDir string
}

// NewDocumentService 创建文档服务
func NewDocumentService(
	embedder *embedding.Client,
	vectorDB vector.VectorClient,
	splitter text.Splitter,
	repo *repository.SQLiteRepository,
) *DocumentService {
	return &DocumentService{
		ctx:       context.Background(),
		embedder:  embedder,
		vectorDB:  vectorDB,
		splitter:  splitter,
		repo:      repo,
		uploadDir: "./data/uploads",
	}
}

// Upload 上传并处理文档
func (s *DocumentService) Upload(file *multipart.FileHeader, title string) (*model.UploadResponse, error) {
	// 1. 保存文件
	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer src.Close()

	// 生成文件名
	ext := filepath.Ext(file.Filename)
	if title == "" {
		title = strings.TrimSuffix(file.Filename, ext)
	}

	filename := fmt.Sprintf("%d_%s%s", file.Size, title, ext)
	filepath := filepath.Join(s.uploadDir, filename)

	// 确保目录存在
	os.MkdirAll(s.uploadDir, 0755)

	// 保存文件
	dst, err := os.Create(filepath)
	if err != nil {
		return nil, fmt.Errorf("创建文件失败: %w", err)
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err != nil {
		return nil, fmt.Errorf("保存文件失败: %w", err)
	}

	// 2. 读取文件内容
	content, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	// 3. 创建文档记录
	doc := &model.Document{
		Title:    title,
		FileName: file.Filename,
		FileSize: file.Size,
		Content:  string(content),
		Status:   model.DocStatusPending,
	}

	docID, err := s.repo.CreateDocument(doc)
	if err != nil {
		return nil, fmt.Errorf("创建文档记录失败: %w", err)
	}
	doc.ID = docID

	// 4. 异步处理向量化
	go s.processDocument(doc)

	return &model.UploadResponse{
		DocumentID: docID,
		Title:     title,
		Chunks:    0,
		Message:   "文档上传成功，正在后台处理向量化...",
	}, nil
}

// processDocument 处理文档向量化
func (s *DocumentService) processDocument(doc *model.Document) {
	log.Printf("[文档 %d] 开始处理向量化...", doc.ID)

	// 更新状态为处理中
	s.repo.UpdateDocumentStatus(doc.ID, model.DocStatusProcessing, "", 0)

	// 1. 文本切片
	chunks := s.splitter.Split(doc.Content)
	log.Printf("[文档 %d] 切片完成，共 %d 个切片", doc.ID, len(chunks))

	if len(chunks) == 0 {
		s.repo.UpdateDocumentStatus(doc.ID, model.DocStatusFailed, "文档为空或切片失败", 0)
		return
	}

	// 2. 生成向量并入库
	var vectorIDs []string
	var chunkModels []model.DocumentChunk

	for i, chunk := range chunks {
		// 生成 Embedding
		vec, err := s.embedder.Embed(ctx, chunk.Content)
		if err != nil {
			log.Printf("[文档 %d] 向量化失败: %v", doc.ID, err)
			continue
		}

		// 插入向量数据库
		id := fmt.Sprintf("doc_%d_chunk_%d", doc.ID, i)
		_, err = s.vectorDB.Insert(ctx, []string{id}, [][]float32{vec})
		if err != nil {
			log.Printf("[文档 %d] 向量入库失败: %v", doc.ID, err)
			continue
		}

		vectorIDs = append(vectorIDs, id)
		chunkModels = append(chunkModels, model.DocumentChunk{
			DocID:      doc.ID,
			Content:    chunk.Content,
			ChunkIndex: i,
			Metadata:   chunk.Metadata,
			VectorID:   id,
		})
	}

	// 3. 保存切片信息到数据库
	for _, chunk := range chunkModels {
		s.repo.CreateChunk(&chunk)
	}

	// 4. 更新文档状态
	s.repo.UpdateDocumentStatus(doc.ID, model.DocStatusCompleted, "", len(chunkModels))
	log.Printf("[文档 %d] 处理完成，共入库 %d 个切片", doc.ID, len(chunkModels))
}

// List 列出文档
func (s *DocumentService) List(page, pageSize int) (*model.DocumentListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	docs, total, err := s.repo.ListDocuments(page, pageSize)
	if err != nil {
		return nil, err
	}

	return &model.DocumentListResponse{
		Total:     total,
		Page:      page,
		PageSize:  pageSize,
		Documents: docs,
	}, nil
}

// Get 获取文档详情
func (s *DocumentService) Get(id int64) (*model.DocumentDetailResponse, error) {
	doc, err := s.repo.GetDocument(id)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("文档不存在")
	}

	chunks, err := s.repo.GetChunksByDocID(id)
	if err != nil {
		return nil, err
	}

	return &model.DocumentDetailResponse{
		Document: *doc,
		Chunks:   chunks,
	}, nil
}

// Delete 删除文档
func (s *DocumentService) Delete(id int64) error {
	// 获取所有切片
	chunks, err := s.repo.GetChunksByDocID(id)
	if err != nil {
		return err
	}

	// 删除向量
	for _, chunk := range chunks {
		if chunk.VectorID != "" {
			s.vectorDB.Delete(ctx, []string{chunk.VectorID})
		}
	}

	// 删除数据库记录（会自动删除切片）
	if err := s.repo.DeleteChunksByDocID(id); err != nil {
		return err
	}
	if err := s.repo.DeleteDocument(id); err != nil {
		return err
	}

	return nil
}