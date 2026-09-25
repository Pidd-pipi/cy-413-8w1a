package model

import "time"

type Journal struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	Title     string    `gorm:"not null" json:"title"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	MoodLevel int       `json:"mood_level"`
	Weather   string    `json:"weather"`
	IsPrivate bool      `gorm:"default:true" json:"is_private"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// MoodTags 仅用于写日记时多选情绪标签，不落 journals 表；
	// 创建当天若无情绪记录，会带着这些标签联动生成一条 Mood。
	MoodTags []string `gorm:"-" json:"mood_tags,omitempty"`
	// LinkedMood 为这篇日记自动创建且仍联动的情绪记录；为 nil 表示没有关联记录。
	LinkedMood *Mood `gorm:"-" json:"linked_mood,omitempty"`
	// DayHasMood 表示日记当天是否存在任意情绪记录（含手动记录），
	// 与 LinkedMood 配合可区分"未关联但当天已有手动记录"的情况。
	DayHasMood bool `gorm:"-" json:"day_has_mood"`
}
