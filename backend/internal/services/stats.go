package services

import (
	"sort"
	"strings"
	"time"

	"qrforqa/internal/models"
	"qrforqa/internal/schema"
)

// OptionStat is one bar of a choice distribution. Percent is of the responses
// that answered the question, not of all responses, so a skipped optional
// question does not dilute its own chart.
type OptionStat struct {
	Value   string  `json:"value"`
	Label   string  `json:"label"`
	Count   int     `json:"count"`
	Percent float64 `json:"percent"`
}

// QuestionStat is the aggregate view of one question.
type QuestionStat struct {
	Key          string       `json:"key"`
	Type         string       `json:"type"`
	Label        string       `json:"label"`
	Answered     int          `json:"answered"`
	Skipped      int          `json:"skipped"`
	Options      []OptionStat `json:"options,omitempty"`
	Average      float64      `json:"average,omitempty"`
	Sum          int          `json:"sum,omitempty"`
	Samples      []string     `json:"samples,omitempty"`
	SampleTotal  int          `json:"sampleTotal,omitempty"`
	Images       []ImageView  `json:"images,omitempty"`
	LongestChars int          `json:"longestChars,omitempty"`
}

// TimelinePoint is one day of submissions.
type TimelinePoint struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// StatsView backs the admin statistics page.
type StatsView struct {
	QuestionnaireID uint           `json:"questionnaireId"`
	Title           string         `json:"title"`
	Total           int            `json:"total"`
	AvgDurationMs   int64          `json:"avgDurationMs"`
	FirstAt         *time.Time     `json:"firstAt"`
	LastAt          *time.Time     `json:"lastAt"`
	Questions       []QuestionStat `json:"questions"`
	Timeline        []TimelinePoint `json:"timeline"`
	TotalImages     int            `json:"totalImages"`
}

// maxSamples caps how many free-text answers travel to the dashboard; a
// thousand essays in one JSON response would stall the browser for no benefit.
const maxSamples = 50

// Stats decodes every response and aggregates it.
//
// Decoding on read is deliberate: storing a decoded copy alongside the
// ciphertext would put plaintext back in the database and defeat the point.
func (s *Service) Stats(q *models.Questionnaire) (*StatsView, error) {
	def, err := s.Definition(q)
	if err != nil {
		return nil, err
	}
	responses, err := s.AllResponses(q)
	if err != nil {
		return nil, err
	}

	view := &StatsView{
		QuestionnaireID: q.ID,
		Title:           def.Title,
		Total:           len(responses),
		Questions:       []QuestionStat{},
		Timeline:        []TimelinePoint{},
	}

	var totalDuration int64
	for i := range responses {
		r := &responses[i]
		totalDuration += r.DurationMs
		view.TotalImages += len(r.Images)
		if view.FirstAt == nil || r.CreatedAt.Before(*view.FirstAt) {
			t := r.CreatedAt
			view.FirstAt = &t
		}
		if view.LastAt == nil || r.CreatedAt.After(*view.LastAt) {
			t := r.CreatedAt
			view.LastAt = &t
		}
	}
	if len(responses) > 0 {
		view.AvgDurationMs = totalDuration / int64(len(responses))
	}

	// Submissions per day, over the whole span including empty days, so the
	// chart does not silently compress gaps.
	if view.FirstAt != nil && view.LastAt != nil {
		counts := map[string]int{}
		for i := range responses {
			counts[responses[i].CreatedAt.Format("2006-01-02")]++
		}
		for d := view.FirstAt.Truncate(24 * time.Hour); !d.After(*view.LastAt); d = d.AddDate(0, 0, 1) {
			key := d.Format("2006-01-02")
			view.Timeline = append(view.Timeline, TimelinePoint{Date: key, Count: counts[key]})
		}
	}

	for _, question := range def.AllQuestions() {
		stat := QuestionStat{Key: question.Key, Type: question.Type, Label: question.Label}

		// Answered always counts RESPONDENTS, never selections. Summing option
		// counts instead (as this did for multi-choice) makes
		// Skipped = total - Answered go negative as soon as anyone picks more
		// than one option.
		for i := range responses {
			if answeredValue(responses[i].Answers[question.Key]) {
				stat.Answered++
			}
		}
		stat.Skipped = len(responses) - stat.Answered

		switch question.Type {
		case schema.TypeSingle, schema.TypeSelect:
			stat.Options = countChoices(responses, question, false)
		case schema.TypeMulti:
			stat.Options = countChoices(responses, question, true)
		case schema.TypeRating:
			dist := map[int]int{}
			sum, n := 0, 0
			for i := range responses {
				v, ok := responses[i].Answers[question.Key]
				if !ok || v == nil {
					continue
				}
				f, ok := v.(float64)
				if !ok {
					continue
				}
				iv := int(f)
				dist[iv]++
				sum += iv
				n++
			}
			stat.Sum = sum
			if n > 0 {
				stat.Average = float64(sum) / float64(n)
			}
			for r := question.Min; r <= question.Max; r++ {
				stat.Options = append(stat.Options, OptionStat{
					Value: itoa(r), Label: itoa(r), Count: dist[r],
					Percent: percent(dist[r], n),
				})
			}
		case schema.TypeText, schema.TypeTextarea:
			samples := []string{}
			for i := range responses {
				v, ok := responses[i].Answers[question.Key]
				if !ok {
					continue
				}
				str, ok := v.(string)
				if !ok || str == "" {
					continue
				}
				if len(str) > stat.LongestChars {
					stat.LongestChars = len(str)
				}
				if len(samples) < maxSamples {
					samples = append(samples, str)
				}
			}
			stat.SampleTotal = stat.Answered
			stat.Samples = samples
		case schema.TypeDate:
			stat.Options = countChoices(responses, question, false)
		case schema.TypeImage:
			seen := map[uint]bool{}
			for i := range responses {
				v, ok := responses[i].Answers[question.Key].([]any)
				if !ok {
					continue
				}
				for _, x := range v {
					if f, ok := x.(float64); ok && f > 0 && !seen[uint(f)] {
						seen[uint(f)] = true
					}
				}
			}
			for i := range responses {
				for _, img := range responses[i].Images {
					if seen[img.ID] && img.FieldKey == question.Key {
						stat.Images = append(stat.Images, img)
					}
				}
			}
			if len(stat.Images) > 60 {
				stat.Images = stat.Images[:60]
			}
		}

		view.Questions = append(view.Questions, stat)
	}

	return view, nil
}

// answeredValue reports whether a response actually answered a question,
// independently of the question's type. Blank strings and empty lists count as
// unanswered, matching the server's own submission validation.
func answeredValue(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(t) != ""
	case []any:
		return len(t) > 0
	case float64:
		return true
	default:
		return false
	}
}

func countChoices(responses []ResponseView, q schema.Question, multi bool) []OptionStat {
	counts := map[string]int{}
	for i := range responses {
		v, ok := responses[i].Answers[q.Key]
		if !ok || v == nil {
			continue
		}
		if multi {
			list, ok := v.([]any)
			if !ok {
				continue
			}
			for _, x := range list {
				if s, ok := x.(string); ok {
					counts[s]++
				}
			}
			continue
		}
		if s, ok := v.(string); ok {
			counts[s]++
		}
	}

	total := 0
	for _, c := range counts {
		total += c
	}
	out := make([]OptionStat, 0, len(q.Options))
	for _, o := range q.Options {
		out = append(out, OptionStat{
			Value: o.Value, Label: o.Label, Count: counts[o.Value],
			Percent: percent(counts[o.Value], total),
		})
	}
	return out
}

func percent(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// LeaderboardEntry powers the dashboard's "most active questionnaires" panel.
type LeaderboardEntry struct {
	ID    uint   `json:"id"`
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Count int64  `json:"count"`
}

// Leaderboard returns the questionnaires with the most responses.
func (s *Service) Leaderboard(limit int) ([]LeaderboardEntry, error) {
	if limit <= 0 || limit > 50 {
		limit = 5
	}
	type row struct {
		ID    uint
		Slug  string
		N     int64
	}
	var rows []row
	if err := s.db.Model(&models.Response{}).
		Select("questionnaires.id as id, questionnaires.slug as slug, count(responses.id) as n").
		Joins("JOIN questionnaires ON questionnaires.id = responses.questionnaire_id").
		Group("questionnaires.id, questionnaires.slug").
		Order("n desc").Limit(limit).Scan(&rows).Error; err != nil {
		return nil, err
	}

	summaries, err := s.List(false)
	if err != nil {
		return nil, err
	}
	titles := make(map[uint]string, len(summaries))
	for _, sm := range summaries {
		titles[sm.ID] = sm.Title
	}

	out := make([]LeaderboardEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, LeaderboardEntry{ID: r.ID, Slug: r.Slug, Title: titles[r.ID], Count: r.N})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out, nil
}
