package model

import "time"

type Mood struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"index;not null" json:"user_id"`
	MoodLevel  int       `gorm:"not null" json:"mood_level"`
	MoodTags   string    `gorm:"type:text;not null" json:"mood_tags"`
	Note       string    `json:"note"`
	RecordDate time.Time `gorm:"index;not null" json:"record_date"`
	// JournalID 标记本条情绪记录由某篇日记自动创建并仍与之联动；
	// 手动创建为 nil；用户在情绪页手动编辑联动记录后置 nil（视为手动维护，日记不再同步）。
	JournalID *uint     `gorm:"index" json:"journal_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
