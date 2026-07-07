package service

import (
	"encoding/json"
	"fmt"
)

// DialogueStatus 对话状态枚举
type DialogueStatus string

const (
	DialogueStatusIdle     DialogueStatus = "idle"     // 空闲状态，无活跃任务
	DialogueStatusActive   DialogueStatus = "active"   // 活跃状态，正在进行任务
	DialogueStatusComplete DialogueStatus = "complete" // 完成状态，任务已完成
)

// TaskType 任务类型枚举
type TaskType string

const (
	TaskTypeOrderQuery    TaskType = "order_query"    // 订单查询任务
	TaskTypeRefundApply   TaskType = "refund_apply"   // 退款申请任务
	TaskTypeProductSearch TaskType = "product_search" // 商品搜索任务
)

// TaskTemplate 任务模板
// 定义任务的步骤、需要收集的数据和提示语
type TaskTemplate struct {
	Type           TaskType                             // 任务类型
	Name           string                               // 任务名称
	Steps          []TaskStep                           // 任务步骤列表
	CompletionFunc func(*DialogueState) (string, error) // 完成回调函数
}

// TaskStep 任务步骤
type TaskStep struct {
	ID          string // 步骤ID
	Description string // 步骤描述
	FieldName   string // 需要收集的字段名
	Prompt      string // 提示用户的话术
	Optional    bool   // 是否可选
}

// DialogueState 对话状态
// 存储当前对话的任务信息和已收集的数据
type DialogueState struct {
	SessionID   string                 // 会话ID
	TaskType    TaskType               // 当前任务类型
	Status      DialogueStatus         // 对话状态
	CurrentStep int                    // 当前步骤索引
	Collected   map[string]interface{} // 已收集的数据
	CreatedAt   string                 // 创建时间
}

// DialogueManager 对话状态管理器
// 负责管理多轮任务式对话的状态流转
// 核心特性：
//   - 支持多种任务类型（订单查询、退款申请等）
//   - 分步收集用户信息
//   - 状态持久化到会话上下文
type DialogueManager struct {
	sessionManager *SessionManager // 会话管理器
	taskTemplates  map[TaskType]*TaskTemplate
}

// NewDialogueManager 创建对话状态管理器实例
func NewDialogueManager(sessionManager *SessionManager) *DialogueManager {
	dm := &DialogueManager{
		sessionManager: sessionManager,
		taskTemplates:  make(map[TaskType]*TaskTemplate),
	}
	dm.registerTaskTemplates()
	return dm
}

// registerTaskTemplates 注册任务模板
func (dm *DialogueManager) registerTaskTemplates() {
	dm.taskTemplates[TaskTypeOrderQuery] = &TaskTemplate{
		Type: TaskTypeOrderQuery,
		Name: "订单查询",
		Steps: []TaskStep{
			{
				ID:          "order_id",
				Description: "获取订单号",
				FieldName:   "order_id",
				Prompt:      "请提供您的订单号",
				Optional:    false,
			},
			{
				ID:          "phone",
				Description: "获取手机号后四位",
				FieldName:   "phone",
				Prompt:      "请提供您的手机号码后四位",
				Optional:    false,
			},
			{
				ID:          "address",
				Description: "获取收货地址",
				FieldName:   "address",
				Prompt:      "请提供收货地址中的街道名称",
				Optional:    true,
			},
		},
		CompletionFunc: dm.completeOrderQuery,
	}

	dm.taskTemplates[TaskTypeRefundApply] = &TaskTemplate{
		Type: TaskTypeRefundApply,
		Name: "退款申请",
		Steps: []TaskStep{
			{
				ID:          "order_id",
				Description: "获取订单号",
				FieldName:   "order_id",
				Prompt:      "请提供需要退款的订单号",
				Optional:    false,
			},
			{
				ID:          "reason",
				Description: "获取退款原因",
				FieldName:   "reason",
				Prompt:      "请说明退款原因",
				Optional:    false,
			},
			{
				ID:          "bank",
				Description: "获取银行卡号",
				FieldName:   "bank_account",
				Prompt:      "请提供接收退款的银行卡号",
				Optional:    false,
			},
		},
		CompletionFunc: dm.completeRefundApply,
	}

	dm.taskTemplates[TaskTypeProductSearch] = &TaskTemplate{
		Type: TaskTypeProductSearch,
		Name: "商品搜索",
		Steps: []TaskStep{
			{
				ID:          "keyword",
				Description: "获取搜索关键词",
				FieldName:   "keyword",
				Prompt:      "请输入您想搜索的商品名称",
				Optional:    false,
			},
			{
				ID:          "category",
				Description: "获取商品分类",
				FieldName:   "category",
				Prompt:      "请选择商品分类（可选）",
				Optional:    true,
			},
		},
		CompletionFunc: dm.completeProductSearch,
	}
}

// StartTask 启动任务
// 参数：
//   - sessionID: 会话ID
//   - taskType: 任务类型
//
// 返回：第一步的提示语
func (dm *DialogueManager) StartTask(sessionID string, taskType TaskType) (string, error) {
	template, ok := dm.taskTemplates[taskType]
	if !ok {
		return "", fmt.Errorf("未知任务类型: %s", taskType)
	}

	state := &DialogueState{
		SessionID:   sessionID,
		TaskType:    taskType,
		Status:      DialogueStatusActive,
		CurrentStep: 0,
		Collected:   make(map[string]interface{}),
	}

	err := dm.saveDialogueState(sessionID, state)
	if err != nil {
		return "", err
	}

	return template.Steps[0].Prompt, nil
}

// ProcessInput 处理用户输入
// 根据当前对话状态，收集信息并推进任务
// 参数：
//   - sessionID: 会话ID
//   - input: 用户输入
//
// 返回：
//   - response: 系统回复
//   - completed: 任务是否完成
func (dm *DialogueManager) ProcessInput(sessionID, input string) (string, bool, error) {
	state, err := dm.GetDialogueState(sessionID)
	if err != nil {
		return "", false, err
	}

	if state == nil || state.Status != DialogueStatusActive {
		return "", false, nil
	}

	template, ok := dm.taskTemplates[state.TaskType]
	if !ok {
		return "", false, fmt.Errorf("未知任务类型: %s", state.TaskType)
	}

	// 收集当前步骤的数据
	currentStep := template.Steps[state.CurrentStep]
	state.Collected[currentStep.FieldName] = input

	// 推进到下一步
	state.CurrentStep++

	if state.CurrentStep >= len(template.Steps) {
		// 任务完成
		state.Status = DialogueStatusComplete
		err := dm.saveDialogueState(sessionID, state)
		if err != nil {
			return "", true, err
		}

		// 执行完成回调
		response, err := template.CompletionFunc(state)
		return response, true, err
	}

	// 更新状态并返回下一步提示
	err = dm.saveDialogueState(sessionID, state)
	if err != nil {
		return "", false, err
	}

	return template.Steps[state.CurrentStep].Prompt, false, nil
}

// GetDialogueState 获取对话状态
func (dm *DialogueManager) GetDialogueState(sessionID string) (*DialogueState, error) {
	session, err := dm.sessionManager.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	if session == nil || session.Context == "" {
		return nil, nil
	}

	var state DialogueState
	err = json.Unmarshal([]byte(session.Context), &state)
	if err != nil {
		return nil, err
	}

	return &state, nil
}

// saveDialogueState 保存对话状态
func (dm *DialogueManager) saveDialogueState(sessionID string, state *DialogueState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	return dm.sessionManager.UpdateSessionContext(sessionID, string(data))
}

// CancelTask 取消当前任务
func (dm *DialogueManager) CancelTask(sessionID string) error {
	state := &DialogueState{
		SessionID: sessionID,
		Status:    DialogueStatusIdle,
	}

	return dm.saveDialogueState(sessionID, state)
}

// ResetDialogue 重置对话状态
func (dm *DialogueManager) ResetDialogue(sessionID string) error {
	return dm.CancelTask(sessionID)
}

// completeOrderQuery 订单查询完成回调
func (dm *DialogueManager) completeOrderQuery(state *DialogueState) (string, error) {
	orderID := state.Collected["order_id"]
	phone := state.Collected["phone"]
	address := state.Collected["address"]

	response := fmt.Sprintf(
		"已收集所有信息，正在查询订单...\n"+
			"订单号: %v\n"+
			"手机号后四位: %v\n"+
			"收货地址: %v\n"+
			"\n查询结果：订单状态为【已发货】，预计3天内送达。",
		orderID, phone, address,
	)

	return response, nil
}

// completeRefundApply 退款申请完成回调
func (dm *DialogueManager) completeRefundApply(state *DialogueState) (string, error) {
	orderID := state.Collected["order_id"]
	reason := state.Collected["reason"]
	bankAccount := state.Collected["bank_account"]

	response := fmt.Sprintf(
		"退款申请已提交！\n"+
			"订单号: %v\n"+
			"退款原因: %v\n"+
			"收款账户: %v\n"+
			"\n我们将在3-5个工作日内处理您的退款申请。",
		orderID, reason, bankAccount,
	)

	return response, nil
}

// completeProductSearch 商品搜索完成回调
func (dm *DialogueManager) completeProductSearch(state *DialogueState) (string, error) {
	keyword := state.Collected["keyword"]
	category := state.Collected["category"]

	response := fmt.Sprintf(
		"正在为您搜索商品...\n"+
			"搜索关键词: %v\n"+
			"商品分类: %v\n"+
			"\n找到以下相关商品：\n"+
			"1. 商品A - 价格: ¥199\n"+
			"2. 商品B - 价格: ¥299\n"+
			"3. 商品C - 价格: ¥399",
		keyword, category,
	)

	return response, nil
}
