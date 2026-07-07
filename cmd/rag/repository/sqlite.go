package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/golllm/cmd/rag/model"

	_ "modernc.org/sqlite"
)

// SQLiteRepository SQLite 数据存储
type SQLiteRepository struct {
	db *sql.DB
}

// NewSQLite 创建 SQLite 存储实例
func NewSQLite(dbPath string) (*SQLiteRepository, error) {
	// 确保目录存在
	// os.MkdirAll(filepath.Dir(dbPath), 0755)

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	repo := &SQLiteRepository{db: db}
	if err := repo.initTables(); err != nil {
		return nil, fmt.Errorf("初始化表失败: %w", err)
	}

	return repo, nil
}

// initTables 初始化数据库表
func (r *SQLiteRepository) initTables() error {
	// 文档表
	_, err := r.db.Exec(`
		CREATE TABLE IF NOT EXISTS documents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			file_name TEXT NOT NULL,
			file_size INTEGER NOT NULL,
			content TEXT NOT NULL,
			chunk_count INTEGER DEFAULT 0,
			status TEXT DEFAULT 'pending',
			error_msg TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("创建文档表失败: %w", err)
	}

	// 文档切片表（子块）
	_, err = r.db.Exec(`
		CREATE TABLE IF NOT EXISTS document_chunks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			doc_id INTEGER NOT NULL,
			parent_id INTEGER DEFAULT 0,
			content TEXT NOT NULL,
			chunk_index INTEGER NOT NULL,
			metadata TEXT,
			vector_id TEXT,
			FOREIGN KEY (doc_id) REFERENCES documents(id) ON DELETE CASCADE,
			FOREIGN KEY (parent_id) REFERENCES parent_chunks(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return fmt.Errorf("创建切片表失败: %w", err)
	}

	// 父块表
	_, err = r.db.Exec(`
		CREATE TABLE IF NOT EXISTS parent_chunks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			doc_id INTEGER NOT NULL,
			content TEXT NOT NULL,
			heading TEXT,
			metadata TEXT,
			FOREIGN KEY (doc_id) REFERENCES documents(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return fmt.Errorf("创建父块表失败: %w", err)
	}

	// 会话表
	_, err = r.db.Exec(`
		CREATE TABLE IF NOT EXISTS chat_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT UNIQUE NOT NULL,
			user_id TEXT NOT NULL DEFAULT '',
			title TEXT DEFAULT '新对话',
			status TEXT DEFAULT 'active',
			context TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("创建会话表失败: %w", err)
	}

	// 消息表
	_, err = r.db.Exec(`
		CREATE TABLE IF NOT EXISTS chat_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			"references" TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES chat_sessions(session_id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return fmt.Errorf("创建消息表失败: %w", err)
	}

	// 创建索引
	r.db.Exec("CREATE INDEX IF NOT EXISTS idx_chunks_doc_id ON document_chunks(doc_id)")
	r.db.Exec("CREATE INDEX IF NOT EXISTS idx_chunks_parent_id ON document_chunks(parent_id)")
	r.db.Exec("CREATE INDEX IF NOT EXISTS idx_parent_chunks_doc_id ON parent_chunks(doc_id)")
	r.db.Exec("CREATE INDEX IF NOT EXISTS idx_messages_session ON chat_messages(session_id)")

	return nil
}

// Close 关闭数据库连接
func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

// ==================== 文档操作 ====================

// CreateDocument 创建文档
func (r *SQLiteRepository) CreateDocument(doc *model.Document) (int64, error) {
	result, err := r.db.Exec(`
		INSERT INTO documents (title, file_name, file_size, content, status)
		VALUES (?, ?, ?, ?, ?)
	`, doc.Title, doc.FileName, doc.FileSize, doc.Content, doc.Status)
	if err != nil {
		return 0, fmt.Errorf("创建文档失败: %w", err)
	}
	return result.LastInsertId()
}

// GetDocument 获取文档
func (r *SQLiteRepository) GetDocument(id int64) (*model.Document, error) {
	doc := &model.Document{}
	err := r.db.QueryRow(`
		SELECT id, title, file_name, file_size, content, chunk_count, status, error_msg, created_at, updated_at
		FROM documents WHERE id = ?
	`, id).Scan(&doc.ID, &doc.Title, &doc.FileName, &doc.FileSize, &doc.Content,
		&doc.ChunkCount, &doc.Status, &doc.ErrorMsg, &doc.CreatedAt, &doc.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询文档失败: %w", err)
	}
	return doc, nil
}

// ListDocuments 列出文档
func (r *SQLiteRepository) ListDocuments(page, pageSize int) ([]model.Document, int64, error) {
	// 获取总数
	var total int64
	r.db.QueryRow("SELECT COUNT(*) FROM documents").Scan(&total)

	// 获取列表
	offset := (page - 1) * pageSize
	rows, err := r.db.Query(`
		SELECT id, title, file_name, file_size, chunk_count, status, created_at, updated_at
		FROM documents ORDER BY created_at DESC LIMIT ? OFFSET ?
	`, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("查询文档列表失败: %w", err)
	}
	defer rows.Close()

	var docs []model.Document
	for rows.Next() {
		var doc model.Document
		rows.Scan(&doc.ID, &doc.Title, &doc.FileName, &doc.FileSize,
			&doc.ChunkCount, &doc.Status, &doc.CreatedAt, &doc.UpdatedAt)
		docs = append(docs, doc)
	}

	return docs, total, nil
}

// UpdateDocumentStatus 更新文档状态
func (r *SQLiteRepository) UpdateDocumentStatus(id int64, status, errorMsg string, chunkCount int) error {
	_, err := r.db.Exec(`
		UPDATE documents SET status = ?, error_msg = ?, chunk_count = ?, updated_at = ?
		WHERE id = ?
	`, status, errorMsg, chunkCount, time.Now(), id)
	return err
}

// DeleteDocument 删除文档
func (r *SQLiteRepository) DeleteDocument(id int64) error {
	_, err := r.db.Exec("DELETE FROM documents WHERE id = ?", id)
	return err
}

// ==================== 切片操作 ====================

// CreateChunk 创建切片
func (r *SQLiteRepository) CreateChunk(chunk *model.DocumentChunk) (int64, error) {
	metadata, _ := json.Marshal(chunk.Metadata)
	result, err := r.db.Exec(`
		INSERT INTO document_chunks (doc_id, parent_id, content, chunk_index, metadata, vector_id)
		VALUES (?, ?, ?, ?, ?, ?)
	`, chunk.DocID, chunk.ParentID, chunk.Content, chunk.ChunkIndex, string(metadata), chunk.VectorID)
	if err != nil {
		return 0, fmt.Errorf("创建切片失败: %w", err)
	}
	return result.LastInsertId()
}

// CreateParentChunk 创建父块
func (r *SQLiteRepository) CreateParentChunk(chunk *model.ParentChunk) (int64, error) {
	metadata, _ := json.Marshal(chunk.Metadata)
	result, err := r.db.Exec(`
		INSERT INTO parent_chunks (doc_id, content, heading, metadata)
		VALUES (?, ?, ?, ?)
	`, chunk.DocID, chunk.Content, chunk.Heading, string(metadata))
	if err != nil {
		return 0, fmt.Errorf("创建父块失败: %w", err)
	}
	return result.LastInsertId()
}

// GetParentChunk 获取父块
func (r *SQLiteRepository) GetParentChunk(parentID int64) (*model.ParentChunk, error) {
	chunk := &model.ParentChunk{}
	var metadataJSON string
	err := r.db.QueryRow(`
		SELECT id, doc_id, content, heading, metadata
		FROM parent_chunks WHERE id = ?
	`, parentID).Scan(&chunk.ID, &chunk.DocID, &chunk.Content, &chunk.Heading, &metadataJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询父块失败: %w", err)
	}
	json.Unmarshal([]byte(metadataJSON), &chunk.Metadata)
	return chunk, nil
}

// GetParentChunksByDocID 获取文档的所有父块
func (r *SQLiteRepository) GetParentChunksByDocID(docID int64) ([]model.ParentChunk, error) {
	rows, err := r.db.Query(`
		SELECT id, doc_id, content, heading, metadata
		FROM parent_chunks WHERE doc_id = ?
	`, docID)
	if err != nil {
		return nil, fmt.Errorf("查询父块失败: %w", err)
	}
	defer rows.Close()

	var chunks []model.ParentChunk
	for rows.Next() {
		var chunk model.ParentChunk
		var metadataJSON string
		rows.Scan(&chunk.ID, &chunk.DocID, &chunk.Content, &chunk.Heading, &metadataJSON)
		json.Unmarshal([]byte(metadataJSON), &chunk.Metadata)
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

// DeleteParentChunksByDocID 删除文档的所有父块
func (r *SQLiteRepository) DeleteParentChunksByDocID(docID int64) error {
	_, err := r.db.Exec("DELETE FROM parent_chunks WHERE doc_id = ?", docID)
	return err
}

// GetChunksByDocID 获取文档的所有切片
func (r *SQLiteRepository) GetChunksByDocID(docID int64) ([]model.DocumentChunk, error) {
	rows, err := r.db.Query(`
		SELECT id, doc_id, parent_id, content, chunk_index, metadata, vector_id
		FROM document_chunks WHERE doc_id = ? ORDER BY chunk_index
	`, docID)
	if err != nil {
		return nil, fmt.Errorf("查询切片失败: %w", err)
	}
	defer rows.Close()

	var chunks []model.DocumentChunk
	for rows.Next() {
		var chunk model.DocumentChunk
		var metadataJSON string
		rows.Scan(&chunk.ID, &chunk.DocID, &chunk.ParentID, &chunk.Content, &chunk.ChunkIndex, &metadataJSON, &chunk.VectorID)
		json.Unmarshal([]byte(metadataJSON), &chunk.Metadata)
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

// DeleteChunksByDocID 删除文档的所有切片
func (r *SQLiteRepository) DeleteChunksByDocID(docID int64) error {
	_, err := r.db.Exec("DELETE FROM document_chunks WHERE doc_id = ?", docID)
	return err
}

// ==================== 会话操作 ====================

// CreateSession 创建会话
func (r *SQLiteRepository) CreateSession(sessionID, userID string) error {
	_, err := r.db.Exec(`
		INSERT INTO chat_sessions (session_id, user_id) VALUES (?, ?)
	`, sessionID, userID)
	return err
}

// GetSession 获取会话
func (r *SQLiteRepository) GetSession(sessionID string) (*model.ChatSession, error) {
	session := &model.ChatSession{}
	err := r.db.QueryRow(`
		SELECT id, session_id, user_id, title, status, context, created_at, updated_at
		FROM chat_sessions WHERE session_id = ?
	`, sessionID).Scan(&session.ID, &session.SessionID, &session.UserID, &session.Title, &session.Status, &session.Context, &session.CreatedAt, &session.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return session, nil
}

// UpdateSessionTitle 更新会话标题
func (r *SQLiteRepository) UpdateSessionTitle(sessionID, title string) error {
	_, err := r.db.Exec(`
		UPDATE chat_sessions SET title = ?, updated_at = ? WHERE session_id = ?
	`, title, time.Now(), sessionID)
	return err
}

// UpdateSessionStatus 更新会话状态
func (r *SQLiteRepository) UpdateSessionStatus(sessionID, status string) error {
	_, err := r.db.Exec(`
		UPDATE chat_sessions SET status = ?, updated_at = ? WHERE session_id = ?
	`, status, time.Now(), sessionID)
	return err
}

// UpdateSessionContext 更新会话上下文
func (r *SQLiteRepository) UpdateSessionContext(sessionID, context string) error {
	_, err := r.db.Exec(`
		UPDATE chat_sessions SET context = ?, updated_at = ? WHERE session_id = ?
	`, context, time.Now(), sessionID)
	return err
}

// ListSessionsByUserID 获取用户的会话列表
func (r *SQLiteRepository) ListSessionsByUserID(userID string, page, pageSize int) ([]model.ChatSession, int64, error) {
	var total int64
	r.db.QueryRow("SELECT COUNT(*) FROM chat_sessions WHERE user_id = ?", userID).Scan(&total)

	offset := (page - 1) * pageSize
	rows, err := r.db.Query(`
		SELECT id, session_id, user_id, title, status, context, created_at, updated_at
		FROM chat_sessions WHERE user_id = ? ORDER BY updated_at DESC LIMIT ? OFFSET ?
	`, userID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("查询会话列表失败: %w", err)
	}
	defer rows.Close()

	var sessions []model.ChatSession
	for rows.Next() {
		var session model.ChatSession
		rows.Scan(&session.ID, &session.SessionID, &session.UserID, &session.Title, &session.Status, &session.Context, &session.CreatedAt, &session.UpdatedAt)
		sessions = append(sessions, session)
	}

	return sessions, total, nil
}

// DeleteSession 删除会话
func (r *SQLiteRepository) DeleteSession(sessionID string) error {
	_, err := r.db.Exec("DELETE FROM chat_sessions WHERE session_id = ?", sessionID)
	return err
}

// GetChatHistory 获取聊天历史
func (r *SQLiteRepository) GetChatHistory(sessionID string) ([]model.ChatMessage, error) {
	rows, err := r.db.Query(`
		SELECT id, session_id, role, content, "references", created_at
		FROM chat_messages WHERE session_id = ? ORDER BY created_at ASC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []model.ChatMessage
	for rows.Next() {
		var msg model.ChatMessage
		var refsJSON string
		rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &refsJSON, &msg.CreatedAt)
		if refsJSON != "" {
			json.Unmarshal([]byte(refsJSON), &msg.References)
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

// CreateMessage 创建消息
func (r *SQLiteRepository) CreateMessage(msg *model.ChatMessage) error {
	refs, _ := json.Marshal(msg.References)
	_, err := r.db.Exec(`
		INSERT INTO chat_messages (session_id, role, content, "references") VALUES (?, ?, ?, ?)
	`, msg.SessionID, msg.Role, msg.Content, string(refs))
	return err
}
