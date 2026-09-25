package service

import (
	"fmt"
	"github.com/blueship581/mindgarden/backend/internal/constants"
	"github.com/blueship581/mindgarden/backend/internal/dto"
	"github.com/blueship581/mindgarden/backend/internal/model"
	"github.com/blueship581/mindgarden/backend/internal/repository"
	"github.com/blueship581/mindgarden/backend/internal/util"
	"log/slog"
)

type JournalService struct {
	repo   repository.JournalRepository
	mood   *MoodService
	logger *slog.Logger
}

func NewJournalService(r repository.JournalRepository, m *MoodService, l *slog.Logger) *JournalService {
	return &JournalService{r, m, l}
}

// journalNote 把日记正文截断为情绪记录备注（备注上限 500 字）。
func journalNote(content string) string {
	const maxRunes = 500
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "…"
}

// normalizeJournalTags 校验日记选择的情绪标签；为空时回退到 calm，非法标签拒绝。
func normalizeJournalTags(tags []string) ([]string, error) {
	if len(tags) == 0 {
		return []string{constants.MoodCalm}, nil
	}
	if !validTags(tags) {
		return nil, util.NewAppError(constants.CodeValidation, "Journal[mood_tags] save failed: unsupported tag", nil)
	}
	return tags, nil
}

func (s *JournalService) Create(uid uint, req dto.JournalRequest) (*model.Journal, error) {
	tags, e := normalizeJournalTags(req.MoodTags)
	if e != nil {
		return nil, e
	}
	v := &model.Journal{UserID: uid, Title: req.Title, Content: req.Content, MoodLevel: req.MoodLevel, MoodTags: tags, Weather: req.Weather, IsPrivate: req.IsPrivate}
	if e = s.repo.Create(v); e != nil {
		return nil, fmt.Errorf("Journal[user_id] create failed: %w", e)
	}
	// 保存新日记：当天还没有情绪记录时，连同指数、标签、日记备注联动生成一条；
	// 当天已有记录（用户手动维护过）则保持不动。
	if req.MoodLevel > 0 {
		if _, e = s.mood.LinkJournalMood(uid, v.ID, req.MoodLevel, tags, journalNote(req.Content), v.CreatedAt); e != nil {
			s.logger.Error(constants.LogErrorWrapped, "stage", "journal create link mood", "journal_id", v.ID, "error", e)
		}
	}
	s.logger.Info(constants.LogJournalCreated, "user_id", uid)
	return v, nil
}
func (s *JournalService) List(uid uint, level int) ([]model.Journal, error) {
	v, e := s.repo.List(uid, level)
	if e != nil {
		return nil, fmt.Errorf("Journal[user_id] list failed: %w", e)
	}
	if e = s.mood.EnrichJournals(uid, v); e != nil {
		return nil, fmt.Errorf("Journal[mood] enrich failed: %w", e)
	}
	s.logger.Info(constants.LogJournalListed, "user_id", uid)
	return v, nil
}
func (s *JournalService) Update(uid, id uint, req dto.JournalRequest) (*model.Journal, error) {
	tags, e := normalizeJournalTags(req.MoodTags)
	if e != nil {
		return nil, e
	}
	v, e := s.repo.ByID(id, uid)
	if e != nil {
		return nil, fmt.Errorf("Journal[id=%d] read failed: %w", id, e)
	}
	v.Title = req.Title
	v.Content = req.Content
	v.MoodLevel = req.MoodLevel
	v.Weather = req.Weather
	v.IsPrivate = req.IsPrivate
	if e = s.repo.Update(v); e != nil {
		return nil, util.WrapEntity("Journal", "content", id, constants.CodeInternal, e)
	}
	// 编辑日记只同步这篇日记自己创建、且用户没在情绪页改过的记录；
	// 手动创建或后来手动编辑过的记录照旧保留，不覆盖。
	if _, e = s.mood.SyncJournalMood(uid, id, req.MoodLevel, tags, journalNote(req.Content)); e != nil {
		s.logger.Error(constants.LogErrorWrapped, "stage", "journal update sync mood", "journal_id", id, "error", e)
	}
	s.logger.Info(constants.LogJournalUpdated, "journal_id", id)
	return v, nil
}
func (s *JournalService) Delete(uid, id uint) error {
	v, e := s.repo.ByID(id, uid)
	if e != nil {
		return fmt.Errorf("Journal[id=%d] read failed: %w", id, e)
	}
	if e = s.repo.Delete(v); e != nil {
		return util.WrapEntity("Journal", "id", id, constants.CodeInternal, e)
	}
	// 日记移除后情绪仍然保留：不删除联动记录，也不主动解除关联。
	s.logger.Info(constants.LogJournalDeleted, "journal_id", id)
	return nil
}
