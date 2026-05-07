package migration

import (
	"testing"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestComponentQueue_PushPop(t *testing.T) {
	queue := NewComponentQueue(10)
	defer queue.Close()

	item := model.MigrationItem{ComponentID: "test1"}
	assert.True(t, queue.Push(item))

	popped, ok := queue.Pop()
	assert.True(t, ok)
	assert.Equal(t, "test1", popped.ComponentID)
}

func TestComponentQueue_TryPush(t *testing.T) {
	queue := NewComponentQueue(2)
	defer queue.Close()

	assert.True(t, queue.TryPush(model.MigrationItem{ComponentID: "1"}))
	assert.True(t, queue.TryPush(model.MigrationItem{ComponentID: "2"}))
	assert.False(t, queue.TryPush(model.MigrationItem{ComponentID: "3"}))
}

func TestComponentQueue_Close(t *testing.T) {
	queue := NewComponentQueue(10)
	queue.Close()

	_, ok := queue.Pop()
	assert.False(t, ok)
}

func TestComponentQueue_Len(t *testing.T) {
	queue := NewComponentQueue(10)
	defer queue.Close()

	assert.Equal(t, 0, queue.Len())
	queue.Push(model.MigrationItem{ComponentID: "1"})
	assert.Equal(t, 1, queue.Len())
	queue.Push(model.MigrationItem{ComponentID: "2"})
	assert.Equal(t, 2, queue.Len())
}

func TestComponentQueue_Cap(t *testing.T) {
	queue := NewComponentQueue(10)
	defer queue.Close()

	assert.Equal(t, 10, queue.Cap())
}
