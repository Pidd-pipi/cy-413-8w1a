package repository

import (
	"errors"
	"github.com/blueship581/mindgarden/backend/internal/model"
	"gorm.io/gorm"
	"time"
)

type MoodRepository interface {
	Create(*model.Mood) error
	List(uint, *time.Time) ([]model.Mood, error)
	ByID(uint, uint) (*model.Mood, error)
	Update(*model.Mood) error
	Delete(*model.Mood) error
	// ListByJournalIDs 返回指定用户、由给定日记集合联动创建的情绪记录（journal_id 非空且命中）。
	ListByJournalIDs(uid uint, journalIDs []uint) ([]model.Mood, error)
	// ListBetween 返回指定用户、记录时间落在 [start, end) 区间内的全部情绪记录。
	ListBetween(uid uint, start, end time.Time) ([]model.Mood, error)
	// ExistsOnDay 判断指定用户在某天（本地零点起的 24 小时窗口）是否已有任意情绪记录。
	ExistsOnDay(uid uint, day time.Time) (bool, error)
}
type moodRepository struct{ db *gorm.DB }

func NewMoodRepository(db *gorm.DB) MoodRepository   { return &moodRepository{db} }
func (r *moodRepository) Create(v *model.Mood) error { return r.db.Create(v).Error }
func (r *moodRepository) List(uid uint, date *time.Time) (out []model.Mood, e error) {
	q := r.db.Where("user_id = ?", uid)
	if date != nil {
		q = q.Where("record_date >= ? AND record_date < ?", date.Truncate(24*time.Hour), date.Truncate(24*time.Hour).AddDate(0, 0, 1))
	}
	e = q.Order("record_date desc, id desc").Find(&out).Error
	return
}
func (r *moodRepository) ListByJournalIDs(uid uint, journalIDs []uint) (out []model.Mood, e error) {
	if len(journalIDs) == 0 {
		return out, nil
	}
	e = r.db.Where("user_id = ? AND journal_id IN ?", uid, journalIDs).Find(&out).Error
	return
}
func (r *moodRepository) ListBetween(uid uint, start, end time.Time) (out []model.Mood, e error) {
	e = r.db.Where("user_id = ? AND record_date >= ? AND record_date < ?", uid, start, end).Find(&out).Error
	return
}
func (r *moodRepository) ExistsOnDay(uid uint, day time.Time) (bool, error) {
	start := day.Truncate(24 * time.Hour)
	var n int64
	e := r.db.Model(&model.Mood{}).
		Where("user_id = ? AND record_date >= ? AND record_date < ?", uid, start, start.AddDate(0, 0, 1)).
		Limit(1).Count(&n).Error
	return n > 0, e
}
func (r *moodRepository) ByID(id, uid uint) (*model.Mood, error) {
	var v model.Mood
	e := r.db.Where("id = ? AND user_id = ?", id, uid).First(&v).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &v, e
}
func (r *moodRepository) Update(v *model.Mood) error { return r.db.Save(v).Error }
func (r *moodRepository) Delete(v *model.Mood) error { return r.db.Delete(v).Error }
