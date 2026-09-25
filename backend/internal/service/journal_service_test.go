package service

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/blueship581/mindgarden/backend/internal/dto"
	"github.com/blueship581/mindgarden/backend/internal/model"
	"github.com/blueship581/mindgarden/backend/internal/repository"
)

// ---- fakes ----

type fakeJournalRepo struct {
	rows map[uint]*model.Journal
	next uint
	uid  uint
}

func (r *fakeJournalRepo) Create(v *model.Journal) error {
	r.next++
	v.ID = r.next
	cp := *v
	r.rows[v.ID] = &cp
	return nil
}
func (r *fakeJournalRepo) List(uid uint, level int) (out []model.Journal, e error) {
	for _, v := range r.rows {
		if v.UserID == uid && (level <= 0 || v.MoodLevel == level) {
			out = append(out, *v)
		}
	}
	return
}
func (r *fakeJournalRepo) ByID(id, uid uint) (*model.Journal, error) {
	v, ok := r.rows[id]
	if !ok || v.UserID != uid {
		return nil, repository.ErrNotFound
	}
	cp := *v
	return &cp, nil
}
func (r *fakeJournalRepo) Update(v *model.Journal) error {
	if _, ok := r.rows[v.ID]; !ok {
		return repository.ErrNotFound
	}
	cp := *v
	r.rows[v.ID] = &cp
	return nil
}
func (r *fakeJournalRepo) Delete(v *model.Journal) error {
	delete(r.rows, v.ID)
	return nil
}

type fakeMoodRepo struct {
	rows map[uint]*model.Mood
	next uint
}

func (r *fakeMoodRepo) Create(v *model.Mood) error {
	r.next++
	v.ID = r.next
	cp := *v
	r.rows[v.ID] = &cp
	return nil
}
func (r *fakeMoodRepo) List(uid uint, date *time.Time) (out []model.Mood, e error) {
	for _, v := range r.rows {
		if v.UserID != uid {
			continue
		}
		if date != nil {
			start := date.UTC().Truncate(24 * time.Hour)
			if v.RecordDate.Before(start) || !v.RecordDate.Before(start.AddDate(0, 0, 1)) {
				continue
			}
		}
		out = append(out, *v)
	}
	return
}
func (r *fakeMoodRepo) ByID(id, uid uint) (*model.Mood, error) {
	v, ok := r.rows[id]
	if !ok || v.UserID != uid {
		return nil, repository.ErrNotFound
	}
	cp := *v
	return &cp, nil
}
func (r *fakeMoodRepo) ByJournalID(uid, journalID uint) (*model.Mood, error) {
	for _, v := range r.rows {
		if v.UserID == uid && v.JournalID != nil && *v.JournalID == journalID {
			cp := *v
			return &cp, nil
		}
	}
	return nil, repository.ErrNotFound
}
func (r *fakeMoodRepo) Update(v *model.Mood) error {
	if _, ok := r.rows[v.ID]; !ok {
		return repository.ErrNotFound
	}
	cp := *v
	r.rows[v.ID] = &cp
	return nil
}
func (r *fakeMoodRepo) Delete(v *model.Mood) error {
	delete(r.rows, v.ID)
	return nil
}
func (r *fakeMoodRepo) UnlinkByJournal(uid, journalID uint) error {
	for _, v := range r.rows {
		if v.UserID == uid && v.JournalID != nil && *v.JournalID == journalID {
			v.JournalID = nil
			v.SyncedFromJournal = false
		}
	}
	return nil
}

// fakeTxScope 始终返回同一组 fake 仓储，等价于在同一事务里读写。
type fakeTxScope struct {
	j *fakeJournalRepo
	m *fakeMoodRepo
}

func (s *fakeTxScope) JournalRepo() repository.JournalRepository { return s.j }
func (s *fakeTxScope) MoodRepo() repository.MoodRepository       { return s.m }

type fakeTx struct {
	j *fakeJournalRepo
	m *fakeMoodRepo
}

func (t *fakeTx) InTx(fn func(repository.TxScope) error) error {
	return fn(&fakeTxScope{j: t.j, m: t.m})
}

func newTestJournalService() (*JournalService, *fakeJournalRepo, *fakeMoodRepo) {
	j := &fakeJournalRepo{rows: map[uint]*model.Journal{}}
	m := &fakeMoodRepo{rows: map[uint]*model.Mood{}}
	s := NewJournalService(j, m, &fakeTx{j: j, m: m}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return s, j, m
}

func journalReq(level int, tags ...string) dto.JournalRequest {
	return dto.JournalRequest{
		Title:     "今天的花园",
		Content:   "下雨了，在家看书。",
		MoodLevel: level,
		MoodTags:  tags,
		Weather:   "雨",
		IsPrivate: true,
	}
}

func parseTags(t *testing.T, raw string) []string {
	t.Helper()
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		t.Fatalf("mood_tags not JSON: %v", err)
	}
	return tags
}

func moodCountForDay(t *testing.T, m *fakeMoodRepo, uid uint) int {
	t.Helper()
	now := time.Now().UTC()
	out, err := m.List(uid, &now)
	if err != nil {
		t.Fatal(err)
	}
	return len(out)
}

// 1. 新建日记，当天无情绪记录：自动生成一条并关联，心情指数/标签/备注来自日记。
func TestJournalCreateAutoCreatesMood(t *testing.T) {
	s, _, m := newTestJournalService()
	v, err := s.Create(1, journalReq(8, "calm", "happy"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if v.Mood == nil {
		t.Fatal("expected linked mood in view")
	}
	if v.Mood.MoodLevel != 8 || !v.Mood.SyncedFromJournal || v.Mood.JournalID == nil || *v.Mood.JournalID != v.ID {
		t.Fatalf("linked mood mismatch: %+v", v.Mood)
	}
	if tags := parseTags(t, v.Mood.MoodTags); len(tags) != 2 || tags[0] != "calm" {
		t.Fatalf("unexpected tags: %v", tags)
	}
	if !strings.Contains(v.Mood.Note, v.Title) || !strings.Contains(v.Mood.Note, v.Content) {
		t.Fatalf("note should reference journal: %q", v.Mood.Note)
	}
	if !v.Mood.RecordDate.Equal(v.Mood.RecordDate.UTC().Truncate(24 * time.Hour)) {
		t.Fatalf("record_date not normalized to day start: %v", v.Mood.RecordDate)
	}
	if moodCountForDay(t, m, 1) != 1 {
		t.Fatal("expected exactly one mood for the day")
	}
}

// 2. 当天已有手动情绪记录：保存日记不覆盖、不新增，视图中不带关联。
func TestJournalCreateKeepsExistingManualMood(t *testing.T) {
	s, _, m := newTestJournalService()
	if _, err := s.Create(1, journalReq(6, "anxious")); err != nil {
		t.Fatal(err)
	}
	first := m.rows[1]

	v, err := s.Create(1, journalReq(9, "happy"))
	if err != nil {
		t.Fatal(err)
	}
	if v.Mood != nil {
		t.Fatal("new journal must not claim the manual mood")
	}
	if moodCountForDay(t, m, 1) != 1 {
		t.Fatal("journal save must not create a second mood for the day")
	}
	if m.rows[first.ID].MoodLevel != 6 || parseTags(t, m.rows[first.ID].MoodTags)[0] != "anxious" {
		t.Fatal("existing mood was overwritten")
	}
}

// 3. 编辑日记：只同步自己创建且未被手动改过的记录。
func TestJournalUpdateSyncsOnlyOwnedUntouchedMood(t *testing.T) {
	s, _, m := newTestJournalService()
	created, err := s.Create(1, journalReq(5, "tired"))
	if err != nil {
		t.Fatal(err)
	}
	jid := created.ID

	updated, err := s.Update(1, jid, journalReq(7, "calm", "happy"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Mood == nil || updated.Mood.MoodLevel != 7 {
		t.Fatalf("synced mood mismatch: %+v", updated.Mood)
	}
	if tags := parseTags(t, updated.Mood.MoodTags); len(tags) != 2 {
		t.Fatalf("synced tags mismatch: %v", tags)
	}

	// 用户随后在情绪记录页手动修改了心情指数，同步标记应被取消。
	ms := NewMoodService(m, slog.New(slog.NewTextHandler(io.Discard, nil)))
	day := time.Now().UTC().Format("2006-01-02")
	mid := updated.Mood.ID
	if _, err := ms.Update(1, mid, dto.MoodRequest{MoodLevel: 2, MoodTags: []string{"angry"}, Note: "我自己改的", RecordDate: day}); err != nil {
		t.Fatal(err)
	}

	// 再次编辑日记，手动改过的记录必须照旧保留。
	if _, err := s.Update(1, jid, journalReq(10, "happy")); err != nil {
		t.Fatal(err)
	}
	got, err := m.ByID(mid, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.MoodLevel != 2 || got.Note != "我自己改的" || got.SyncedFromJournal {
		t.Fatalf("manual mood was overwritten by journal edit: %+v", got)
	}
	if got.JournalID == nil || *got.JournalID != jid {
		t.Fatal("journal link should remain for timeline display")
	}
}

// 4. 当天已有手动记录的日记被编辑时：不补建情绪记录。
func TestJournalUpdateNeverBackfillsMood(t *testing.T) {
	s, _, m := newTestJournalService()
	_ = m.Create(&model.Mood{UserID: 1, MoodLevel: 4, MoodTags: `["anxious"]`, RecordDate: time.Now().UTC().Truncate(24 * time.Hour)})
	created, err := s.Create(1, journalReq(8, "happy"))
	if err != nil {
		t.Fatal(err)
	}
	updated, err := s.Update(1, created.ID, journalReq(9, "calm"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Mood != nil {
		t.Fatal("must not associate the manual mood")
	}
	if moodCountForDay(t, m, 1) != 1 {
		t.Fatal("must not backfill a mood on update")
	}
}

// 5. 删除日记：情绪记录保留，仅解除关联。
func TestJournalDeleteKeepsMood(t *testing.T) {
	s, j, m := newTestJournalService()
	created, err := s.Create(1, journalReq(7, "calm"))
	if err != nil {
		t.Fatal(err)
	}
	mid := created.Mood.ID
	if err := s.Delete(1, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := j.ByID(created.ID, 1); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("journal should be deleted, got %v", err)
	}
	kept, err := m.ByID(mid, 1)
	if err != nil {
		t.Fatalf("mood must be retained after journal delete: %v", err)
	}
	if kept.JournalID != nil {
		t.Fatal("journal link must be cleared")
	}
}

// 6. 非法标签在日记创建/编辑时被拒绝。
func TestJournalRejectsUnsupportedTags(t *testing.T) {
	s, _, _ := newTestJournalService()
	if _, err := s.Create(1, journalReq(7, "dreamy")); err == nil {
		t.Fatal("unsupported tag should fail create")
	}
	created, err := s.Create(1, journalReq(7, "calm"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(1, created.ID, journalReq(7, "dreamy")); err == nil {
		t.Fatal("unsupported tag should fail update")
	}
}

// 7. 列表按 journal_id 附带关联情绪，无关联时 mood 为空。
func TestJournalListIncludesLinkedMood(t *testing.T) {
	s, _, _ := newTestJournalService()
	linked, err := s.Create(1, journalReq(7, "calm"))
	if err != nil {
		t.Fatal(err)
	}
	// 同一用户再写一篇同日日记：当天已有自动情绪，不关联。
	plain, err := s.Create(1, journalReq(8, "happy"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.List(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	seenLinked, seenPlain := false, false
	for _, v := range list {
		if v.ID == linked.ID {
			seenLinked = true
			if v.Mood == nil || v.Mood.ID != linked.Mood.ID {
				t.Fatal("linked journal should carry mood")
			}
		}
		if v.ID == plain.ID {
			seenPlain = true
			if v.Mood != nil {
				t.Fatal("journal created with same-day mood present must show no link")
			}
		}
	}
	if !seenLinked || !seenPlain {
		t.Fatalf("list missing journals: %+v", list)
	}
}

// 8. 情绪备注超长时按 500 字截断。
func TestJournalMoodNoteTruncated(t *testing.T) {
	long := strings.Repeat("内", 600)
	note := journalMoodNote("标题", long)
	if len([]rune(note)) > moodNoteMax {
		t.Fatalf("note length %d exceeds %d", len([]rune(note)), moodNoteMax)
	}
}

// 9. 编辑日记时心情指数为 0（历史数据/旧客户端）：不动关联情绪，避免违反 1-10 约束。
func TestJournalUpdateWithZeroLevelSkipsMoodSync(t *testing.T) {
	s, _, m := newTestJournalService()
	created, err := s.Create(1, journalReq(7, "calm"))
	if err != nil {
		t.Fatal(err)
	}
	mid := created.Mood.ID
	req := journalReq(0, "happy")
	req.Content = "改成没有心情指数的日记"
	updated, err := s.Update(1, created.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Mood == nil || updated.Mood.MoodLevel != 7 {
		t.Fatalf("linked mood level must stay 7, got %+v", updated.Mood)
	}
	got, err := m.ByID(mid, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.MoodLevel != 7 || parseTags(t, got.MoodTags)[0] != "calm" {
		t.Fatalf("linked mood must not be synced with zero level: %+v", got)
	}
}
