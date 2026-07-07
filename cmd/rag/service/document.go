package service

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/golllm/cmd/rag/model"
	"github.com/golllm/cmd/rag/repository"
	"github.com/golllm/pkg/embedding"
	"github.com/golllm/pkg/text"
	"github.com/golllm/pkg/vector"
)

// DocumentService 文档服务，负责文档上传、处理、管理和查询
// 主要功能：
//   - 文档上传：接收用户上传的文件，保存到本地并创建文档记录
//   - 语义切片：使用语义相似度算法将文档切分为父子块结构
//   - 向量化入库：将子块向量化并存入向量数据库
//   - 文档管理：支持文档列表、详情查询和删除操作
type DocumentService struct {
	ctx           context.Context              // 全局上下文
	embedder      *embedding.Client            // Embedding客户端，用于文本向量化
	vectorDB      vector.VectorClient          // 向量数据库客户端，用于向量存储和检索
	semanticSplit *text.SemanticSplitter       // 语义切片器，基于相似度进行智能切片
	repo          *repository.SQLiteRepository // 数据仓储，用于文档和切片的持久化
	uploadDir     string                       // 文件上传目录
}

// NewDocumentService 创建文档服务实例
// 参数：
//   - embedder: Embedding客户端
//   - vectorDB: 向量数据库客户端
//   - repo: SQLite数据仓储
func NewDocumentService(
	embedder *embedding.Client,
	vectorDB vector.VectorClient,
	repo *repository.SQLiteRepository,
) *DocumentService {
	// 封装Embedding函数，适配语义切片器的接口
	embedFunc := func(texts []string) ([][]float32, error) {
		var result [][]float32
		for _, text := range texts {
			vec, err := embedder.Embedding(context.Background(), text)
			if err != nil {
				return nil, err
			}
			result = append(result, vec)
		}
		return result, nil
	}

	return &DocumentService{
		ctx:           context.Background(),
		embedder:      embedder,
		vectorDB:      vectorDB,
		semanticSplit: text.NewSemanticSplitter(embedFunc),
		repo:          repo,
		uploadDir:     "./data/uploads",
	}
}

// Upload 上传文档并创建文档记录
// 流程：
//  1. 打开上传的文件
//  2. 保存文件到本地目录
//  3. 读取文件内容
//  4. 创建文档记录到数据库
//  5. 启动后台协程处理文档向量化
//
// 参数：
//   - file: 上传的文件头信息
//   - title: 文档标题（可选，默认为文件名）
//
// 返回：
//   - UploadResponse: 上传响应，包含文档ID和处理状态
func (s *DocumentService) Upload(file *multipart.FileHeader, title string) (*model.UploadResponse, error) {
	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer src.Close()

	ext := filepath.Ext(file.Filename)
	if title == "" {
		title = strings.TrimSuffix(file.Filename, ext)
	}

	// 生成唯一文件名，避免冲突
	filename := fmt.Sprintf("%d_%s%s", file.Size, title, ext)
	filepath := filepath.Join(s.uploadDir, filename)

	// 确保上传目录存在
	os.MkdirAll(s.uploadDir, 0755)

	// 创建文件并保存
	dst, err := os.Create(filepath)
	if err != nil {
		return nil, fmt.Errorf("创建文件失败: %w", err)
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err != nil {
		return nil, fmt.Errorf("保存文件失败: %w", err)
	}

	// 读取文件内容
	content, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	// 创建文档记录
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

	// 启动后台协程处理文档向量化，不阻塞上传响应
	go s.processDocument(doc)

	return &model.UploadResponse{
		DocumentID: docID,
		Title:      title,
		Chunks:     0,
		Message:    "文档上传成功，正在后台处理向量化...",
	}, nil
}

// processDocument 处理文档的语义切片和向量化（后台异步执行）
// 流程：
//  1. 更新文档状态为处理中
//  2. 使用语义切片器将文档切分为父子块
//  3. 创建父块记录到数据库
//  4. 遍历子块：创建记录→向量化→向量入库
//  5. 更新文档状态为完成
//
// 参数：
//   - doc: 待处理的文档对象
func (s *DocumentService) processDocument(doc *model.Document) {
	log.Printf("[文档 %d] 开始处理向量化...", doc.ID)

	// 更新文档状态为处理中
	s.repo.UpdateDocumentStatus(doc.ID, model.DocStatusProcessing, "", 0)

	// 使用语义切片器进行智能切片，生成父子块结构
	semanticChunks, err := s.semanticSplit.Split(doc.Content)
	if err != nil {
		log.Printf("[文档 %d] 语义切片失败: %v", doc.ID, err)
		s.repo.UpdateDocumentStatus(doc.ID, model.DocStatusFailed, fmt.Sprintf("语义切片失败: %v", err), 0)
		return
	}

	log.Printf("[文档 %d] 语义切片完成，共 %d 个父块", doc.ID, len(semanticChunks))

	if len(semanticChunks) == 0 {
		s.repo.UpdateDocumentStatus(doc.ID, doc.Status, "文档为空或切片失败", 0)
		return
	}

	totalChunks := 0

	// 遍历每个父块，创建父块记录和子块记录
	for i, semChunk := range semanticChunks {
		// 创建父块记录，存储完整的语义单元内容
		parentChunk := &model.ParentChunk{
			DocID:    doc.ID,
			Content:  semChunk.ParentContent,
			Heading:  semChunk.Heading,
			Metadata: map[string]interface{}{"chunk_index": i},
		}

		parentID, err := s.repo.CreateParentChunk(parentChunk)
		if err != nil {
			log.Printf("[文档 %d] 创建父块失败: %v", doc.ID, err)
			continue
		}

		// 遍历父块下的每个子块
		for _, childContent := range semChunk.ChildContents {
			chunkModel := &model.DocumentChunk{
				DocID:      doc.ID,
				ParentID:   parentID,
				Content:    childContent,
				ChunkIndex: totalChunks,
			}

			// 创建子块记录，获取数据库实际ID
			chunkID, err := s.repo.CreateChunk(chunkModel)
			if err != nil {
				log.Printf("[文档 %d] 创建子块失败: %v", doc.ID, err)
				continue
			}

			// 子块向量化
			vec, err := s.embedder.Embedding(s.ctx, childContent)
			if err != nil {
				log.Printf("[文档 %d] 子块向量化失败: %v", doc.ID, err)
				continue
			}

			// 构建向量元数据，包含文档ID、子块ID、父块ID等
			meta := map[string]interface{}{
				"doc_id":      doc.ID,
				"chunk_id":    chunkID,
				"parent_id":   parentID,
				"chunk_index": totalChunks,
				"file_name":   doc.FileName,
				"heading":     semChunk.Heading,
			}

			// 向量入库
			err = s.vectorDB.Insert(s.ctx, chunkID, vec, childContent, meta)
			if err != nil {
				log.Printf("[文档 %d] 向量入库失败: %v", doc.ID, err)
				continue
			}

			totalChunks++
		}
	}

	// 更新文档状态为完成，并记录切片数量
	s.repo.UpdateDocumentStatus(doc.ID, model.DocStatusCompleted, "", totalChunks)
	log.Printf("[文档 %d] 处理完成，共入库 %d 个子块，%d 个父块", doc.ID, totalChunks, len(semanticChunks))
}

// List 获取文档列表
// 参数：
//   - page: 页码（从1开始）
//   - pageSize: 每页数量
//
// 返回：
//   - DocumentListResponse: 文档列表响应，包含文档列表和总数
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
// 参数：
//   - id: 文档ID
//
// 返回：
//   - DocumentDetailResponse: 文档详情响应，包含文档信息和切片列表
func (s *DocumentService) Get(id int64) (*model.DocumentDetailResponse, error) {
	doc, err := s.repo.GetDocument(id)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("文档不存在")
	}

	// 获取文档的所有切片
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
// 流程：
//  1. 获取文档的所有切片
//  2. 从向量数据库中删除对应的向量
//  3. 删除文档的所有子块记录
//  4. 删除文档的所有父块记录
//  5. 删除文档记录
//
// 参数：
//   - id: 文档ID
func (s *DocumentService) Delete(id int64) error {
	// 获取文档的所有切片
	chunks, err := s.repo.GetChunksByDocID(id)
	if err != nil {
		return err
	}

	// 收集需要删除的向量ID
	var vecIDs []int64
	for _, chunk := range chunks {
		if vecID, parseErr := strconv.ParseInt(chunk.VectorID, 10, 64); parseErr == nil {
			vecIDs = append(vecIDs, vecID)
		}
	}
	// 从向量数据库中删除向量
	if len(vecIDs) > 0 {
		s.vectorDB.Delete(s.ctx, vecIDs)
	}

	// 删除子块记录
	if err := s.repo.DeleteChunksByDocID(id); err != nil {
		return err
	}
	// 删除父块记录
	if err := s.repo.DeleteParentChunksByDocID(id); err != nil {
		return err
	}
	// 删除文档记录
	if err := s.repo.DeleteDocument(id); err != nil {
		return err
	}

	return nil
}
