package service

import (
	"encoding/json"
	"fmt"
	"github.com/blueship581/mindgarden/backend/internal/constants"
	"github.com/blueship581/mindgarden/backend/internal/dto"
	"github.com/blueship581/mindgarden/backend/internal/model"
	"github.com/blueship581/mindgarden/backend/internal/repository"
	"github.com/blueship581/mindgarden/backend/internal/util"
	"log/slog"
	"strings"
	"time"
)

type MoodService struct {
	repo   repository.MoodRepository
	logger *slog.Logger
}

func NewMoodService(r repository.MoodRepository, l *slog.Logger) *MoodService {
	return &MoodService{r, l}
}
func validTags(tags []string) bool {
	allowed := map[string]bool{}
	for _, v := range constants.MoodTags {
		allowed[v] = true
	}
	for _, v := range tags {
		if !allowed[v] {
			return false
		}
	}
	return true
}
func encodeTags(tags []string) string {
	b, _ := json.Marshal(tags)
	return string(b)
}

// LinkJournalMood 由日记触发：在当天还没有任何情绪记录时，用日记的心情指数、标签和备注
// 创建一条与日记联动的情绪记录（journal_id 指向该日记）。
func (s *MoodService) LinkJournalMood(uid, journalID uint, level int, tags []string, note string, day time.Time) (*model.Mood, error) {
	exists, e := s.repo.ExistsOnDay(uid, day)
	if e != nil {
		return nil, fmt.Errorf("Mood[user_id] day lookup failed: %w", e)
	}
	// 当天已有（手动或其它日记联动的）记录：视为用户已维护，绝不覆盖。
	if exists {
		return nil, nil
	}
	v := &model.Mood{
		UserID:     uid,
		MoodLevel:  level,
		MoodTags:   encodeTags(tags),
		Note:       strings.TrimSpace(note),
		RecordDate: day.Truncate(24 * time.Hour),
		JournalID:  &journalID,
	}
	if e = s.repo.Create(v); e != nil {
		return nil, fmt.Errorf("Mood[journal_id=%d] link create failed: %w", journalID, e)
	}
	s.logger.Info(constants.LogMoodLinkedFromJournal, "user_id", uid, "journal_id", journalID, "mood_level", level)
	return v, nil
}

// SyncJournalMood 编辑日记时仅同步这篇日记自己创建、且用户未手动改过的情绪记录。
// 返回被同步的记录；当天没有联动记录（被手动改过/从未生成）时返回 nil，不做任何写入。
func (s *MoodService) SyncJournalMood(uid, journalID uint, level int, tags []string, note string) (*model.Mood, error) {
	if level <= 0 {
		return nil, nil
	}
	vs, e := s.repo.ListByJournalIDs(uid, []uint{journalID})
	if e != nil {
		return nil, fmt.Errorf("Mood[journal_id=%d] link lookup failed: %w", journalID, e)
	}
	if len(vs) == 0 {
		return nil, nil
	}
	v := &vs[0]
	v.MoodLevel = level
	v.MoodTags = encodeTags(tags)
	v.Note = strings.TrimSpace(note)
	if e = s.repo.Update(v); e != nil {
		return nil, fmt.Errorf("Mood[journal_id=%d] link sync failed: %w", journalID, e)
	}
	s.logger.Info(constants.LogMoodSyncedFromJournal, "mood_id", v.ID, "journal_id", journalID)
	return v, nil
}

// EnrichJournals 批量填充日记的联动情绪与"当天是否存在情绪记录"标记，避免 N+1 查询。
func (s *MoodService) EnrichJournals(uid uint, journals []model.Journal) error {
	if len(journals) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(journals))
	for i := range journals {
		ids = append(ids, journals[i].ID)
	}
	linked, e := s.repo.ListByJournalIDs(uid, ids)
	if e != nil {
		return fmt.Errorf("Mood[journal_ids] link lookup failed: %w", e)
	}
	linkedByJournal := map[uint]*model.Mood{}
	dayHasMood := map[string]bool{}
	for i := range linked {
		if linked[i].JournalID != nil {
			linkedByJournal[*linked[i].JournalID] = &linked[i]
		}
		dayHasMood[linked[i].RecordDate.Format("2006-01-02")] = true
	}
	// 还需确认没有联动记录的日记当天是否存在手动记录：按日记覆盖的日期范围一次拉取。
	var earliest, latest time.Time
	for i := range journals {
		d := journals[i].CreatedAt.Truncate(24 * time.Hour)
		if i == 0 || d.Before(earliest) {
			earliest = d
		}
		if d.After(latest) {
			latest = d
		}
	}
	inRange, e := s.repo.ListBetween(uid, earliest, latest.AddDate(0, 0, 1))
	if e != nil {
		return fmt.Errorf("Mood[record_date] range lookup failed: %w", e)
	}
	for i := range inRange {
		dayHasMood[inRange[i].RecordDate.Format("2006-01-02")] = true
	}
	for i := range journals {
		journals[i].LinkedMood = linkedByJournal[journals[i].ID]
		journals[i].DayHasMood = dayHasMood[journals[i].CreatedAt.Format("2006-01-02")]
	}
	return nil
}
func (s *MoodService) Create(uid uint, req dto.MoodRequest) (*model.Mood, error) {
	if !validTags(req.MoodTags) {
		return nil, util.NewAppError(constants.CodeValidation, "Mood[mood_tags] create failed: unsupported tag", nil)
	}
	d, e := time.Parse("2006-01-02", req.RecordDate)
	if e != nil {
		return nil, util.NewAppError(constants.CodeValidation, "Mood[record_date] create failed: invalid date", e)
	}
	v := &model.Mood{UserID: uid, MoodLevel: req.MoodLevel, MoodTags: encodeTags(req.MoodTags), Note: strings.TrimSpace(req.Note), RecordDate: d}
	if e = s.repo.Create(v); e != nil {
		return nil, fmt.Errorf("Mood[user_id] create failed: %w", e)
	}
	s.logger.Info(constants.LogMoodCreated, "user_id", uid, "mood_level", v.MoodLevel)
	return v, nil
}
func (s *MoodService) List(uid uint, date string) ([]model.Mood, error) {
	var d *time.Time
	if date != "" {
		x, e := time.Parse("2006-01-02", date)
		if e != nil {
			return nil, util.NewAppError(constants.CodeValidation, "Mood[record_date] list failed: invalid date", e)
		}
		d = &x
	}
	vs, e := s.repo.List(uid, d)
	if e != nil {
		return nil, fmt.Errorf("Mood[user_id] list failed: %w", e)
	}
	s.logger.Info(constants.LogMoodListed, "user_id", uid)
	return vs, nil
}
func (s *MoodService) Update(uid, id uint, req dto.MoodRequest) (*model.Mood, error) {
	v, e := s.repo.ByID(id, uid)
	if e != nil {
		return nil, fmt.Errorf("Mood[id=%d] fetch failed: %w", id, e)
	}
	if !validTags(req.MoodTags) {
		return nil, util.WrapEntity("Mood", "mood_tags", id, constants.CodeValidation, nil)
	}
	d, e := time.Parse("2006-01-02", req.RecordDate)
	if e != nil {
		return nil, util.WrapEntity("Mood", "record_date", id, constants.CodeValidation, e)
	}
	// 用户在情绪页手动编辑了一条原本由日记联动的记录：从此视为用户手动维护，
	// 日记后续保存/编辑不再同步它（日记也不会覆盖当天记录）。
	if v.JournalID != nil {
		v.JournalID = nil
		s.logger.Info(constants.LogMoodDetachedFromJournal, "mood_id", id)
	}
	v.MoodLevel = req.MoodLevel
	v.MoodTags = encodeTags(req.MoodTags)
	v.Note = req.Note
	v.RecordDate = d
	if e = s.repo.Update(v); e != nil {
		return nil, util.WrapEntity("Mood", "mood_level", id, constants.CodeInternal, e)
	}
	s.logger.Info(constants.LogMoodUpdated, "mood_id", id)
	return v, nil
}
func (s *MoodService) Delete(uid, id uint) error {
	v, e := s.repo.ByID(id, uid)
	if e != nil {
		return fmt.Errorf("Mood[id=%d] fetch failed: %w", id, e)
	}
	if e = s.repo.Delete(v); e != nil {
		return util.WrapEntity("Mood", "id", id, constants.CodeInternal, e)
	}
	s.logger.Info(constants.LogMoodDeleted, "mood_id", id)
	return nil
}
