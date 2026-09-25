package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/blueship581/mindgarden/backend/internal/constants"
	"github.com/blueship581/mindgarden/backend/internal/dto"
	"github.com/blueship581/mindgarden/backend/internal/model"
	"github.com/blueship581/mindgarden/backend/internal/repository"
	"github.com/blueship581/mindgarden/backend/internal/util"
)

type JournalService struct {
	journalRepo repository.JournalRepository
	moodRepo    repository.MoodRepository
	tx          repository.TxManager
	logger      *slog.Logger
}

func NewJournalService(jr repository.JournalRepository, mr repository.MoodRepository, tx repository.TxManager, l *slog.Logger) *JournalService {
	return &JournalService{journalRepo: jr, moodRepo: mr, tx: tx, logger: l}
}

// moodNoteMax 与 MoodRequest.Note 的长度上限保持一致。
const moodNoteMax = 500

// journalMoodNote 用日记标题与正文生成情绪备注，超长时按 rune 截断。
func journalMoodNote(title, content string) string {
	prefix := "来自日记《" + strings.TrimSpace(title) + "》："
	body := strings.TrimSpace(content)
	if r := []rune(prefix + body); len(r) > moodNoteMax {
		keep := moodNoteMax - len([]rune(prefix))
		if keep < 0 {
			keep = 0
		}
		bodyRunes := []rune(body)
		if keep > len(bodyRunes) {
			keep = len(bodyRunes)
		}
		body = string(bodyRunes[:keep])
	}
	return prefix + body
}

// dayStart 按 UTC 归一化到当天零点，与 mood 仓储的日期区间查询保持一致。
func dayStart(t time.Time) time.Time {
	return t.UTC().Truncate(24 * time.Hour)
}

func marshalMoodTags(tags []string) string {
	if len(tags) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(tags)
	return string(b)
}

// hasMoodForDay 判断用户在某天是否已经存在情绪记录（手动维护过则不再自动生成）。
func hasMoodForDay(mr repository.MoodRepository, uid uint, day time.Time) (bool, error) {
	existing, e := mr.List(uid, &day)
	if e != nil {
		return false, e
	}
	return len(existing) > 0, nil
}

func (s *JournalService) Create(uid uint, req dto.JournalRequest) (*dto.JournalView, error) {
	if len(req.MoodTags) > 0 && !ValidMoodTags(req.MoodTags) {
		return nil, util.NewAppError(constants.CodeValidation, "Journal[mood_tags] create failed: unsupported tag", nil)
	}
	tagsJSON := marshalMoodTags(req.MoodTags)
	var out *dto.JournalView
	e := s.tx.InTx(func(scope repository.TxScope) error {
		jr, mr := scope.JournalRepo(), scope.MoodRepo()
		now := time.Now().UTC()
		j := &model.Journal{
			UserID:    uid,
			Title:     req.Title,
			Content:   req.Content,
			MoodLevel: req.MoodLevel,
			MoodTags:  tagsJSON,
			Weather:   req.Weather,
			IsPrivate: req.IsPrivate,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if e := jr.Create(j); e != nil {
			return fmt.Errorf("Journal[user_id] create failed: %w", e)
		}
		var linked *model.Mood
		// 仅当当天还没有情绪记录，且日记填写了心情指数时，才随日记自动生成一条。
		if req.MoodLevel > 0 {
			exists, e := hasMoodForDay(mr, uid, now)
			if e != nil {
				return fmt.Errorf("Mood[record_date] lookup failed: %w", e)
			}
			if !exists {
				day := dayStart(now)
				m := &model.Mood{
					UserID:            uid,
					MoodLevel:         req.MoodLevel,
					MoodTags:          tagsJSON,
					Note:              journalMoodNote(req.Title, req.Content),
					RecordDate:        day,
					JournalID:         &j.ID,
					SyncedFromJournal: true,
					CreatedAt:         now,
				}
				if e := mr.Create(m); e != nil {
					return fmt.Errorf("Mood[user_id] create failed: %w", e)
				}
				s.logger.Info(constants.LogMoodCreatedFromJournal, "user_id", uid, "journal_id", j.ID)
				linked = m
			} else {
				s.logger.Info(constants.LogMoodKeptManual, "user_id", uid, "journal_id", j.ID)
			}
		}
		out = &dto.JournalView{Journal: *j, Mood: linked}
		return nil
	})
	if e != nil {
		return nil, e
	}
	s.logger.Info(constants.LogJournalCreated, "user_id", uid)
	return out, nil
}

func (s *JournalService) List(uid uint, level int) ([]dto.JournalView, error) {
	journals, e := s.journalRepo.List(uid, level)
	if e != nil {
		return nil, fmt.Errorf("Journal[user_id] list failed: %w", e)
	}
	// 时间轴只需要日记关联的情绪记录，取全量后按 journal_id 建立索引即可。
	moods, e := s.moodRepo.List(uid, nil)
	if e != nil {
		return nil, fmt.Errorf("Mood[user_id] list failed: %w", e)
	}
	linked := map[uint]model.Mood{}
	for _, m := range moods {
		if m.JournalID != nil {
			linked[*m.JournalID] = m
		}
	}
	out := make([]dto.JournalView, 0, len(journals))
	for _, j := range journals {
		v := dto.JournalView{Journal: j}
		if m, ok := linked[j.ID]; ok {
			v.Mood = &m
		}
		out = append(out, v)
	}
	s.logger.Info(constants.LogJournalListed, "user_id", uid)
	return out, nil
}

func (s *JournalService) Update(uid, id uint, req dto.JournalRequest) (*dto.JournalView, error) {
	if len(req.MoodTags) > 0 && !ValidMoodTags(req.MoodTags) {
		return nil, util.WrapEntity("Journal", "mood_tags", id, constants.CodeValidation, nil)
	}
	tagsJSON := marshalMoodTags(req.MoodTags)
	var out *dto.JournalView
	e := s.tx.InTx(func(scope repository.TxScope) error {
		jr, mr := scope.JournalRepo(), scope.MoodRepo()
		j, e := jr.ByID(id, uid)
		if e != nil {
			return fmt.Errorf("Journal[id=%d] read failed: %w", id, e)
		}
		j.Title = req.Title
		j.Content = req.Content
		j.MoodLevel = req.MoodLevel
		j.MoodTags = tagsJSON
		j.Weather = req.Weather
		j.IsPrivate = req.IsPrivate
		if e = jr.Update(j); e != nil {
			return util.WrapEntity("Journal", "content", id, constants.CodeInternal, e)
		}
		var linked *model.Mood
		m, e := mr.ByJournalID(uid, id)
		switch {
		case e == nil:
			// 只同步这篇日记自己创建、之后没有被手动改过、且仍有有效心情指数的情绪记录。
			if m.SyncedFromJournal && req.MoodLevel > 0 {
				m.MoodLevel = req.MoodLevel
				m.MoodTags = tagsJSON
				m.Note = journalMoodNote(req.Title, req.Content)
				if e = mr.Update(m); e != nil {
					return util.WrapEntity("Mood", "journal_id", id, constants.CodeInternal, e)
				}
				s.logger.Info(constants.LogMoodSyncedFromJournal, "mood_id", m.ID, "journal_id", id)
			} else {
				s.logger.Info(constants.LogMoodKeptManual, "mood_id", m.ID, "journal_id", id)
			}
			linked = m
		case errors.Is(e, repository.ErrNotFound):
			// 原本就没有关联记录（当天已有手动记录等情况），不补建。
		default:
			return fmt.Errorf("Mood[journal_id=%d] read failed: %w", id, e)
		}
		out = &dto.JournalView{Journal: *j, Mood: linked}
		return nil
	})
	if e != nil {
		return nil, e
	}
	s.logger.Info(constants.LogJournalUpdated, "journal_id", id)
	return out, nil
}

func (s *JournalService) Delete(uid, id uint) error {
	e := s.tx.InTx(func(scope repository.TxScope) error {
		jr, mr := scope.JournalRepo(), scope.MoodRepo()
		j, e := jr.ByID(id, uid)
		if e != nil {
			return fmt.Errorf("Journal[id=%d] read failed: %w", id, e)
		}
		// 先解除情绪记录的关联（情绪本身保留），再删除日记。
		if e = mr.UnlinkByJournal(uid, id); e != nil {
			return util.WrapEntity("Mood", "journal_id", id, constants.CodeInternal, e)
		}
		if e = jr.Delete(j); e != nil {
			return util.WrapEntity("Journal", "id", id, constants.CodeInternal, e)
		}
		return nil
	})
	if e != nil {
		return e
	}
	s.logger.Info(constants.LogJournalDeleted, "journal_id", id)
	return nil
}
