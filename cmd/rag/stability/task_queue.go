package stability

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TaskStatus 任务状态枚举
type TaskStatus string

const (
	// StatusPending 等待中，任务已创建但尚未开始执行
	StatusPending TaskStatus = "pending"
	// StatusRunning 运行中，任务正在执行
	StatusRunning TaskStatus = "running"
	// StatusCompleted 已完成，任务执行成功
	StatusCompleted TaskStatus = "completed"
	// StatusFailed 失败，任务执行失败
	StatusFailed TaskStatus = "failed"
	// StatusRetrying 重试中，任务失败后正在重试
	StatusRetrying TaskStatus = "retrying"
	// StatusDeadLetter 死信队列，任务超过最大重试次数仍失败
	StatusDeadLetter TaskStatus = "dead_letter"
)

// TaskType 任务类型枚举
type TaskType string

const (
	// TaskTypeVectorize 文档向量化任务
	TaskTypeVectorize TaskType = "vectorize"
	// TaskTypeSummarize 文档摘要任务
	TaskTypeSummarize TaskType = "summarize"
	// TaskTypeTranslate 文档翻译任务
	TaskTypeTranslate TaskType = "translate"
)

// Task 异步任务
type Task struct {
	ID           string      // 任务唯一标识
	Type         TaskType    // 任务类型
	Payload      interface{} // 任务负载数据
	Status       TaskStatus  // 当前状态
	Progress     int         // 进度百分比（0-100）
	ErrorMessage string      // 错误信息
	RetryCount   int         // 已重试次数
	CreatedAt    time.Time   // 创建时间
	UpdatedAt    time.Time   // 更新时间
}

// TaskQueueConfig 任务队列配置参数
type TaskQueueConfig struct {
	Enabled           bool          // 是否启用异步任务队列
	WorkerCount       int           // 工作线程数
	MaxRetries        int           // 最大重试次数
	RetryDelay        time.Duration // 重试间隔
	QueueCapacity     int           // 队列容量
	DeadLetterEnabled bool          // 是否启用死信队列
}

// TaskHandler 任务处理器函数
// 参数：
//   - ctx: 上下文
//   - task: 任务对象
//
// 返回：
//   - error: 错误信息
type TaskHandler func(ctx context.Context, task *Task) error

// TaskQueue 异步任务队列
// 支持任务状态查询、失败重试、死信队列等功能
type TaskQueue struct {
	config     TaskQueueConfig          // 配置参数
	tasks      sync.Map                 // 任务存储（key: taskID）
	queue      chan *Task               // 任务队列
	deadLetter chan *Task               // 死信队列
	handlers   map[TaskType]TaskHandler // 任务处理器映射
	wg         sync.WaitGroup           // 等待组，用于优雅关闭
	running    bool                     // 是否运行中
	mu         sync.RWMutex             // 读写锁
}

// NewTaskQueue 创建任务队列实例
func NewTaskQueue(config TaskQueueConfig) *TaskQueue {
	tq := &TaskQueue{
		config:     config,
		queue:      make(chan *Task, config.QueueCapacity),
		deadLetter: make(chan *Task, config.QueueCapacity),
		handlers:   make(map[TaskType]TaskHandler),
		running:    true,
	}

	if config.WorkerCount <= 0 {
		config.WorkerCount = 4
	}

	for i := 0; i < config.WorkerCount; i++ {
		tq.wg.Add(1)
		go tq.worker(i)
	}

	return tq
}

// RegisterHandler 注册任务处理器
// 参数：
//   - taskType: 任务类型
//   - handler: 任务处理器函数
func (tq *TaskQueue) RegisterHandler(taskType TaskType, handler TaskHandler) {
	tq.mu.Lock()
	defer tq.mu.Unlock()
	tq.handlers[taskType] = handler
}

// Submit 提交任务到队列
// 参数：
//   - taskType: 任务类型
//   - payload: 任务负载数据
//
// 返回：
//   - string: 任务ID
//   - error: 错误信息
func (tq *TaskQueue) Submit(taskType TaskType, payload interface{}) (string, error) {
	if !tq.config.Enabled {
		return "", errors.New("task queue is disabled")
	}

	task := &Task{
		ID:        uuid.New().String(),
		Type:      taskType,
		Payload:   payload,
		Status:    StatusPending,
		Progress:  0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	tq.tasks.Store(task.ID, task)

	select {
	case tq.queue <- task:
		return task.ID, nil
	default:
		return "", errors.New("task queue is full")
	}
}

// worker 工作线程
// 从队列中获取任务并执行
func (tq *TaskQueue) worker(id int) {
	defer tq.wg.Done()

	for {
		select {
		case task := <-tq.queue:
			tq.executeTask(task)
		default:
			tq.mu.RLock()
			if !tq.running {
				tq.mu.RUnlock()
				return
			}
			tq.mu.RUnlock()
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// executeTask 执行任务
// 调用对应的任务处理器，处理失败重试逻辑
func (tq *TaskQueue) executeTask(task *Task) {
	tq.updateStatus(task.ID, StatusRunning)

	handler, ok := tq.handlers[task.Type]
	if !ok {
		tq.updateStatus(task.ID, StatusFailed)
		tq.setErrorMessage(task.ID, "no handler registered for task type: "+string(task.Type))
		return
	}

	for {
		err := handler(context.Background(), task)
		if err == nil {
			tq.updateStatus(task.ID, StatusCompleted)
			tq.setProgress(task.ID, 100)
			return
		}

		task.RetryCount++
		tq.setErrorMessage(task.ID, err.Error())
		tq.setProgress(task.ID, 0)

		if task.RetryCount > tq.config.MaxRetries {
			if tq.config.DeadLetterEnabled {
				tq.updateStatus(task.ID, StatusDeadLetter)
				tq.deadLetter <- task
			} else {
				tq.updateStatus(task.ID, StatusFailed)
			}
			return
		}

		tq.updateStatus(task.ID, StatusRetrying)
		time.Sleep(tq.config.RetryDelay)
	}
}

// GetTask 查询任务状态
// 参数：
//   - taskID: 任务ID
//
// 返回：
//   - *Task: 任务对象
//   - bool: 是否存在
func (tq *TaskQueue) GetTask(taskID string) (*Task, bool) {
	if v, ok := tq.tasks.Load(taskID); ok {
		return v.(*Task), true
	}
	return nil, false
}

// updateStatus 更新任务状态
func (tq *TaskQueue) updateStatus(taskID string, status TaskStatus) {
	if v, ok := tq.tasks.Load(taskID); ok {
		task := v.(*Task)
		task.Status = status
		task.UpdatedAt = time.Now()
		tq.tasks.Store(taskID, task)
	}
}

// setProgress 更新任务进度
func (tq *TaskQueue) setProgress(taskID string, progress int) {
	if v, ok := tq.tasks.Load(taskID); ok {
		task := v.(*Task)
		task.Progress = progress
		task.UpdatedAt = time.Now()
		tq.tasks.Store(taskID, task)
	}
}

// setErrorMessage 更新任务错误信息
func (tq *TaskQueue) setErrorMessage(taskID string, errorMessage string) {
	if v, ok := tq.tasks.Load(taskID); ok {
		task := v.(*Task)
		task.ErrorMessage = errorMessage
		task.UpdatedAt = time.Now()
		tq.tasks.Store(taskID, task)
	}
}

// Close 关闭任务队列
// 等待所有工作线程完成后退出
func (tq *TaskQueue) Close() {
	tq.mu.Lock()
	tq.running = false
	tq.mu.Unlock()

	tq.wg.Wait()
	close(tq.queue)
	close(tq.deadLetter)
}

// GetStats 获取任务队列统计信息
// 返回：
//   - map[string]int: 统计数据
func (tq *TaskQueue) GetStats() map[string]int {
	stats := map[string]int{
		"pending":     0,
		"running":     0,
		"completed":   0,
		"failed":      0,
		"retrying":    0,
		"dead_letter": 0,
	}

	tq.tasks.Range(func(key, value interface{}) bool {
		task := value.(*Task)
		switch task.Status {
		case StatusPending:
			stats["pending"]++
		case StatusRunning:
			stats["running"]++
		case StatusCompleted:
			stats["completed"]++
		case StatusFailed:
			stats["failed"]++
		case StatusRetrying:
			stats["retrying"]++
		case StatusDeadLetter:
			stats["dead_letter"]++
		}
		return true
	})

	return stats
}
