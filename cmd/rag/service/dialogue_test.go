package service

import (
	"testing"

	"github.com/golllm/cmd/rag/repository"
)

func TestDialogueManager_StartTask(t *testing.T) {
	repo, err := repository.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	sm := NewSessionManager(nil, repo, 3600, 100)
	dm := NewDialogueManager(sm)

	err = sm.CreateSession("test-dialogue-start", "user-dialogue")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	response, err := dm.StartTask("test-dialogue-start", TaskTypeOrderQuery)
	if err != nil {
		t.Errorf("StartTask failed: %v", err)
	}

	if response == "" {
		t.Error("StartTask should return non-empty response")
	}

	if response != "请提供您的订单号" {
		t.Errorf("Expected '请提供您的订单号', got '%s'", response)
	}
}

func TestDialogueManager_StartTask_Unknown(t *testing.T) {
	repo, err := repository.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	sm := NewSessionManager(nil, repo, 3600, 100)
	dm := NewDialogueManager(sm)

	err = sm.CreateSession("test-dialogue-unknown", "user-dialogue")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	_, err = dm.StartTask("test-dialogue-unknown", "unknown_task")
	if err == nil {
		t.Error("StartTask should fail for unknown task type")
	}
}

func TestDialogueManager_ProcessInput_OrderQuery(t *testing.T) {
	repo, err := repository.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	sm := NewSessionManager(nil, repo, 3600, 100)
	dm := NewDialogueManager(sm)

	err = sm.CreateSession("test-dialogue-process", "user-dialogue")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	_, err = dm.StartTask("test-dialogue-process", TaskTypeOrderQuery)
	if err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	response1, completed1, err := dm.ProcessInput("test-dialogue-process", "ORD12345")
	if err != nil {
		t.Errorf("ProcessInput step 1 failed: %v", err)
	}
	if completed1 {
		t.Error("Task should not be completed after step 1")
	}
	if response1 != "请提供您的手机号码后四位" {
		t.Errorf("Step 1 response mismatch: got '%s'", response1)
	}

	response2, completed2, err := dm.ProcessInput("test-dialogue-process", "1234")
	if err != nil {
		t.Errorf("ProcessInput step 2 failed: %v", err)
	}
	if completed2 {
		t.Error("Task should not be completed after step 2")
	}
	if response2 != "请提供收货地址中的街道名称" {
		t.Errorf("Step 2 response mismatch: got '%s'", response2)
	}

	response3, completed3, err := dm.ProcessInput("test-dialogue-process", "科技路88号")
	if err != nil {
		t.Errorf("ProcessInput step 3 failed: %v", err)
	}
	if !completed3 {
		t.Error("Task should be completed after step 3")
	}
	if response3 == "" {
		t.Error("Step 3 response should not be empty")
	}
}

func TestDialogueManager_ProcessInput_RefundApply(t *testing.T) {
	repo, err := repository.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	sm := NewSessionManager(nil, repo, 3600, 100)
	dm := NewDialogueManager(sm)

	err = sm.CreateSession("test-dialogue-refund", "user-dialogue")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	_, err = dm.StartTask("test-dialogue-refund", TaskTypeRefundApply)
	if err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	_, completed1, err := dm.ProcessInput("test-dialogue-refund", "ORD67890")
	if err != nil {
		t.Errorf("ProcessInput step 1 failed: %v", err)
	}
	if completed1 {
		t.Error("Task should not be completed after step 1")
	}

	_, completed2, err := dm.ProcessInput("test-dialogue-refund", "商品质量问题")
	if err != nil {
		t.Errorf("ProcessInput step 2 failed: %v", err)
	}
	if completed2 {
		t.Error("Task should not be completed after step 2")
	}

	response3, completed3, err := dm.ProcessInput("test-dialogue-refund", "6222021234567890")
	if err != nil {
		t.Errorf("ProcessInput step 3 failed: %v", err)
	}
	if !completed3 {
		t.Error("Task should be completed after step 3")
	}
	if response3 == "" {
		t.Error("Step 3 response should not be empty")
	}
}

func TestDialogueManager_ProcessInput_ProductSearch(t *testing.T) {
	repo, err := repository.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	sm := NewSessionManager(nil, repo, 3600, 100)
	dm := NewDialogueManager(sm)

	err = sm.CreateSession("test-dialogue-search", "user-dialogue")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	_, err = dm.StartTask("test-dialogue-search", TaskTypeProductSearch)
	if err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	_, completed1, err := dm.ProcessInput("test-dialogue-search", "手机")
	if err != nil {
		t.Errorf("ProcessInput step 1 failed: %v", err)
	}
	if completed1 {
		t.Error("Task should not be completed after step 1")
	}

	response2, completed2, err := dm.ProcessInput("test-dialogue-search", "电子产品")
	if err != nil {
		t.Errorf("ProcessInput step 2 failed: %v", err)
	}
	if !completed2 {
		t.Error("Task should be completed after step 2")
	}
	if response2 == "" {
		t.Error("Step 2 response should not be empty")
	}
}

func TestDialogueManager_GetDialogueState(t *testing.T) {
	repo, err := repository.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	sm := NewSessionManager(nil, repo, 3600, 100)
	dm := NewDialogueManager(sm)

	err = sm.CreateSession("test-dialogue-state", "user-dialogue")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	state, err := dm.GetDialogueState("test-dialogue-state")
	if err != nil {
		t.Errorf("GetDialogueState failed: %v", err)
	}
	if state != nil {
		t.Error("Dialogue state should be nil initially")
	}

	_, err = dm.StartTask("test-dialogue-state", TaskTypeOrderQuery)
	if err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	state, err = dm.GetDialogueState("test-dialogue-state")
	if err != nil {
		t.Errorf("GetDialogueState after start failed: %v", err)
	}
	if state == nil {
		t.Error("Dialogue state should not be nil after starting task")
	}
	if state.TaskType != TaskTypeOrderQuery {
		t.Errorf("TaskType mismatch: expected %s, got %s", TaskTypeOrderQuery, state.TaskType)
	}
	if state.Status != DialogueStatusActive {
		t.Errorf("Status mismatch: expected %s, got %s", DialogueStatusActive, state.Status)
	}
}

func TestDialogueManager_CancelTask(t *testing.T) {
	repo, err := repository.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	sm := NewSessionManager(nil, repo, 3600, 100)
	dm := NewDialogueManager(sm)

	err = sm.CreateSession("test-dialogue-cancel", "user-dialogue")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	_, err = dm.StartTask("test-dialogue-cancel", TaskTypeOrderQuery)
	if err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	err = dm.CancelTask("test-dialogue-cancel")
	if err != nil {
		t.Errorf("CancelTask failed: %v", err)
	}

	state, err := dm.GetDialogueState("test-dialogue-cancel")
	if err != nil {
		t.Errorf("GetDialogueState after cancel failed: %v", err)
	}
	if state == nil {
		t.Error("Dialogue state should not be nil after cancel")
	}
	if state.Status != DialogueStatusIdle {
		t.Errorf("Status should be idle after cancel, got %s", state.Status)
	}
}

func TestDialogueManager_ResetDialogue(t *testing.T) {
	repo, err := repository.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	sm := NewSessionManager(nil, repo, 3600, 100)
	dm := NewDialogueManager(sm)

	err = sm.CreateSession("test-dialogue-reset", "user-dialogue")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	_, err = dm.StartTask("test-dialogue-reset", TaskTypeOrderQuery)
	if err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	err = dm.ResetDialogue("test-dialogue-reset")
	if err != nil {
		t.Errorf("ResetDialogue failed: %v", err)
	}

	state, err := dm.GetDialogueState("test-dialogue-reset")
	if err != nil {
		t.Errorf("GetDialogueState after reset failed: %v", err)
	}
	if state != nil && state.Status != DialogueStatusIdle {
		t.Errorf("Status should be idle after reset, got %s", state.Status)
	}
}

func TestDialogueManager_ProcessInput_NoActiveTask(t *testing.T) {
	repo, err := repository.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	sm := NewSessionManager(nil, repo, 3600, 100)
	dm := NewDialogueManager(sm)

	err = sm.CreateSession("test-dialogue-no-task", "user-dialogue")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	response, completed, err := dm.ProcessInput("test-dialogue-no-task", "Hello")
	if err != nil {
		t.Errorf("ProcessInput should not fail for no active task: %v", err)
	}
	if completed {
		t.Error("Task should not be completed when no task is active")
	}
	if response != "" {
		t.Errorf("Response should be empty when no task is active, got '%s'", response)
	}
}
