package dto

import "github.com/blueship581/mindgarden/backend/internal/model"

// JournalView 在日记数据上附带由该日记创建的情绪记录，供时间轴展示关联状态。
type JournalView struct {
	model.Journal
	Mood *model.Mood `json:"mood,omitempty"`
}
