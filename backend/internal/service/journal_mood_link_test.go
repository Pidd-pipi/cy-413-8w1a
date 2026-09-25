package service

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/blueship581/mindgarden/backend/internal/dto"
	"github.com/blueship581/mindgarden/backend/internal/model"
	"github.com/blueship581/mindgarden/backend/internal/repository"
)

type fakeMoodRepo struct {
	moods []model.Mood
	next  uint
}

func (f *fakeMoodRepo) Create(v *model.Mood) error {
	f.next++
	v.ID = f.next
	f.moods = append(f.moods, *v)
	return nil
}
func (f *fakeMoodRepo) List(_ uint, date *time.Time) (out []model.Mood, e error) {
	for _, m := range f.moods {
		if date != nil {
			start := date.Truncate(24 * time.Hour)
			if m.RecordDate.Before(start) || !m.RecordDate.Before(start.AddDate(0, 0, 1)) {
				continue
			}
		}
		out = append(out, m)
	}
	return
}
func (f *fakeMoodRepo) ListBetween(_ uint, start, end time.Time) (out []model.Mood, e error) {
	for _, m := range f.moods {
		if !m.RecordDate.Before(start) && m.RecordDate.Before(end) {
			out = append(out, m)
		}
	}
	return
}
func (f *fakeMoodRepo) ByID(id, _ uint) (*model.Mood, error) {
	for i := range f.moods {
		if f.moods[i].ID == id {
			return &f.moods[i], nil
		}
	}
	return nil, repository.ErrNotFound
}
func (f *fakeMoodRepo) Update(v *model.Mood) error {
	for i := range f.moods {
		if f.moods[i].ID == v.ID {
			f.moods[i] = *v
			return nil
		}
	}
	return repository.ErrNotFound
}
func (f *fakeMoodRepo) Delete(v *model.Mood) error {
	for i := range f.moods {
		if f.moods[i].ID == v.ID {
			f.moods = append(f.moods[:i], f.moods[i+1:]...)
			return nil
		}
	}
	return repository.ErrNotFound
}
func (f *fakeMoodRepo) ListByJournalIDs(_ uint, ids []uint) (out []model.Mood, e error) {
	want := map[uint]bool{}
	for _, id := range ids {
		want[id] = true
	}
	for _, m := range f.moods {
		if m.JournalID != nil && want[*m.JournalID] {
			out = append(out, m)
		}
	}
	return
}
func (f *fakeMoodRepo) ExistsOnDay(_ uint, day time.Time) (bool, error) {
	start := day.Truncate(24 * time.Hour)
	for _, m := range f.moods {
		if !m.RecordDate.Before(start) && m.RecordDate.Before(start.AddDate(0, 0, 1)) {
			return true, nil
		}
	}
	return false, nil
}

type fakeJournalRepo struct {
	journals []model.Journal
	next     uint
}

func (f *fakeJournalRepo) Create(v *model.Journal) error {
	f.next++
	v.ID = f.next
	v.CreatedAt = time.Date(2026, 9, 25, 21, 0, 0, 0, time.Local)
	f.journals = append(f.journals, *v)
	return nil
}
func (f *fakeJournalRepo) List(_ uint, _ int) (out []model.Journal, e error) {
	return f.journals, nil
}
func (f *fakeJournalRepo) ByID(id, _ uint) (*model.Journal, error) {
	for i := range f.journals {
		if f.journals[i].ID == id {
			return &f.journals[i], nil
		}
	}
	return nil, repository.ErrNotFound
}
func (f *fakeJournalRepo) Update(v *model.Journal) error {
	for i := range f.journals {
		if f.journals[i].ID == v.ID {
			f.journals[i] = *v
			return nil
		}
	}
	return repository.ErrNotFound
}
func (f *fakeJournalRepo) Delete(v *model.Journal) error {
	for i := range f.journals {
		if f.journals[i].ID == v.ID {
			f.journals = append(f.journals[:i], f.journals[i+1:]...)
			return nil
		}
	}
	return repository.ErrNotFound
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func journalReq(level int, tags []string) dto.JournalRequest {
	return dto.JournalRequest{Title: "标题", Content: "今天的正文", MoodLevel: level, MoodTags: tags, Weather: "晴", IsPrivate: true}
}

// 保存新日记：当天无记录时联动生成情绪；当天已有记录时不覆盖。
func TestJournalCreateLinksMoodOnlyWhenDayEmpty(t *testing.T) {
	day := time.Date(2026, 9, 25, 21, 0, 0, 0, time.Local)
	cases := []struct {
		name      string
		seed      *model.Mood
		wantMoods int
		linked    bool
	}{
		{name: "empty day links", seed: nil, wantMoods: 1, linked: true},
		{name: "existing manual record untouched", seed: &model.Mood{UserID: 1, MoodLevel: 3, MoodTags: `["anxious"]`, RecordDate: day}, wantMoods: 1, linked: false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			mr := &fakeMoodRepo{}
			if tt.seed != nil {
				if e := mr.Create(tt.seed); e != nil {
					t.Fatal(e)
				}
			}
			ms := NewMoodService(mr, testLogger())
			js := NewJournalService(&fakeJournalRepo{}, ms, testLogger())
			if _, e := js.Create(1, journalReq(8, []string{"happy", "calm"})); e != nil {
				t.Fatalf("create journal: %v", e)
			}
			if len(mr.moods) != tt.wantMoods {
				t.Fatalf("moods=%d want %d", len(mr.moods), tt.wantMoods)
			}
			gotLinked := mr.moods[len(mr.moods)-1].JournalID != nil
			if gotLinked != tt.linked {
				t.Fatalf("journal_id linked=%v want %v", gotLinked, tt.linked)
			}
			if tt.linked {
				m := mr.moods[len(mr.moods)-1]
				if m.MoodLevel != 8 || m.MoodTags != `["happy","calm"]` || m.Note != "今天的正文" {
					t.Fatalf("linked mood not built from journal: %+v", m)
				}
				if m.RecordDate != day.Truncate(24*time.Hour) {
					t.Fatalf("record_date=%v want midnight", m.RecordDate)
				}
			}
		})
	}
}

// 编辑日记只同步本日记创建的记录；被手动解除关联（手动改过）的记录不动。
func TestJournalUpdateSyncsOnlyOwnLinkedMood(t *testing.T) {
	mr := &fakeMoodRepo{}
	ms := NewMoodService(mr, testLogger())
	jr := &fakeJournalRepo{}
	js := NewJournalService(jr, ms, testLogger())

	j, e := js.Create(1, journalReq(6, []string{"tired"}))
	if e != nil {
		t.Fatal(e)
	}
	if len(mr.moods) != 1 {
		t.Fatalf("expected linked mood, got %d", len(mr.moods))
	}
	// 编辑日记：联动记录应被同步。
	if _, e = js.Update(1, j.ID, journalReq(9, []string{"happy"})); e != nil {
		t.Fatalf("update journal: %v", e)
	}
	m := mr.moods[0]
	if m.MoodLevel != 9 || m.MoodTags != `["happy"]` {
		t.Fatalf("linked mood not synced: %+v", m)
	}

	// 用户在情绪页手动编辑 -> 解除关联；再编辑日记不得覆盖。
	if _, e = ms.Update(1, m.ID, dto.MoodRequest{MoodLevel: 2, MoodTags: []string{"angry"}, Note: "手动备注", RecordDate: "2026-09-25"}); e != nil {
		t.Fatalf("manual mood update: %v", e)
	}
	if mr.moods[0].JournalID != nil {
		t.Fatalf("journal_id should be detached after manual edit")
	}
	if _, e = js.Update(1, j.ID, journalReq(10, []string{"calm"})); e != nil {
		t.Fatalf("update journal after detach: %v", e)
	}
	if mr.moods[0].MoodLevel != 2 || mr.moods[0].Note != "手动备注" {
		t.Fatalf("manually maintained mood was overwritten: %+v", mr.moods[0])
	}
}

// 删除日记后情绪记录仍然保留。
func TestJournalDeleteKeepsMood(t *testing.T) {
	mr := &fakeMoodRepo{}
	ms := NewMoodService(mr, testLogger())
	jr := &fakeJournalRepo{}
	js := NewJournalService(jr, ms, testLogger())
	j, e := js.Create(1, journalReq(6, []string{"calm"}))
	if e != nil {
		t.Fatal(e)
	}
	if e = js.Delete(1, j.ID); e != nil {
		t.Fatalf("delete journal: %v", e)
	}
	if len(mr.moods) != 1 {
		t.Fatalf("mood should be retained after journal deletion, got %d", len(mr.moods))
	}
	if mr.moods[0].JournalID == nil || *mr.moods[0].JournalID != j.ID {
		t.Fatalf("retained mood should keep its journal link marker")
	}
}

// 列表需要带出联动情绪及"当天是否有记录"标记。
func TestJournalListEnrichedWithMoodState(t *testing.T) {
	mr := &fakeMoodRepo{}
	ms := NewMoodService(mr, testLogger())
	jr := &fakeJournalRepo{}
	js := NewJournalService(jr, ms, testLogger())

	linked, e := js.Create(1, journalReq(6, []string{"calm"}))
	if e != nil {
		t.Fatal(e)
	}
	// 手动记录占据另一天，再写该天的日记：不应联动。
	if e = mr.Create(&model.Mood{UserID: 1, MoodLevel: 4, MoodTags: `["anxious"]`, RecordDate: time.Date(2026, 9, 20, 0, 0, 0, 0, time.Local)}); e != nil {
		t.Fatal(e)
	}
	jr.next++
	blocked := linked.ID + 1
	jr.journals = append(jr.journals, model.Journal{ID: blocked, UserID: 1, Title: "另一天", Content: "x", CreatedAt: time.Date(2026, 9, 20, 10, 0, 0, 0, time.Local)})

	out, e := js.List(1, 0)
	if e != nil {
		t.Fatalf("list: %v", e)
	}
	got := map[uint]model.Journal{}
	for _, j := range out {
		got[j.ID] = j
	}
	if got[linked.ID].LinkedMood == nil || !got[linked.ID].DayHasMood {
		t.Fatalf("linked journal should expose linked mood: %+v", got[linked.ID])
	}
	if got[blocked].LinkedMood != nil || !got[blocked].DayHasMood {
		t.Fatalf("blocked journal should show day_has_mood without link: %+v", got[blocked])
	}
}
