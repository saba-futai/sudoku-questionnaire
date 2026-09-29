// Package schema defines the questionnaire definition format and validates
// both definitions and submitted answers.
//
// Validation runs on the DECODED values, so a malformed submission is rejected
// before its ciphertext is ever stored.
package schema

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Question types.
const (
	TypeSingle   = "single"
	TypeMulti    = "multi"
	TypeSelect   = "select"
	TypeText     = "text"
	TypeTextarea = "textarea"
	TypeRating   = "rating"
	TypeDate     = "date"
	TypeImage    = "image"
)

var choiceTypes = map[string]bool{TypeSingle: true, TypeMulti: true, TypeSelect: true}

// QuestionTypes lists the supported types, for the admin builder UI.
var QuestionTypes = []string{
	TypeSingle, TypeMulti, TypeSelect, TypeText, TypeTextarea, TypeRating, TypeDate, TypeImage,
}

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type Question struct {
	Key       string   `json:"key"`
	Type      string   `json:"type"`
	Label     string   `json:"label"`
	Help      string   `json:"help,omitempty"`
	Required  bool     `json:"required"`
	Options   []Option `json:"options,omitempty"`
	MinLength int      `json:"minLength,omitempty"`
	MaxLength int      `json:"maxLength,omitempty"`
	Min       int      `json:"min,omitempty"`
	Max       int      `json:"max,omitempty"`
	MaxImages int      `json:"maxImages,omitempty"`
}

type Step struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Questions   []Question `json:"questions"`
}

// Definition is the whole questionnaire, stored as ciphertext.
type Definition struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Steps       []Step `json:"steps"`
}

var keyRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// MaxSteps/MaxQuestions bound a definition so a hostile payload cannot make
// decoding or rendering quadratic.
const (
	MaxSteps     = 50
	MaxQuestions = 500
)

// Validate checks a definition before it is stored.
func (d *Definition) Validate() error {
	d.Title = strings.TrimSpace(d.Title)
	if d.Title == "" {
		return errors.New("标题不能为空")
	}
	if utf8.RuneCountInString(d.Title) > 200 {
		return errors.New("标题过长(最多 200 字)")
	}
	if len(d.Steps) == 0 {
		return errors.New("至少需要一个步骤")
	}
	if len(d.Steps) > MaxSteps {
		return fmt.Errorf("步骤过多(最多 %d 个)", MaxSteps)
	}

	seen := map[string]bool{}
	total := 0
	for si := range d.Steps {
		s := &d.Steps[si]
		s.Title = strings.TrimSpace(s.Title)
		if s.Title == "" {
			return fmt.Errorf("第 %d 个步骤缺少标题", si+1)
		}
		if len(s.Questions) == 0 {
			return fmt.Errorf("步骤「%s」至少需要一道题", s.Title)
		}
		for qi := range s.Questions {
			q := &s.Questions[qi]
			total++
			q.Label = strings.TrimSpace(q.Label)
			if !keyRe.MatchString(q.Key) {
				return fmt.Errorf("题目 %d 的标识非法(仅允许字母、数字、_ 和 -,最多 64 字符)", qt(qi))
			}
			if seen[q.Key] {
				return fmt.Errorf("题目标识重复:%s", q.Key)
			}
			seen[q.Key] = true
			if q.Label == "" {
				return fmt.Errorf("题目「%s」缺少标题", q.Key)
			}
			if !validType(q.Type) {
				return fmt.Errorf("题目「%s」的类型未知:%s", q.Key, q.Type)
			}
			if choiceTypes[q.Type] {
				if len(q.Options) == 0 {
					return fmt.Errorf("题目「%s」是选择题但没有选项", q.Key)
				}
				optSeen := map[string]bool{}
				for oi := range q.Options {
					o := &q.Options[oi]
					o.Value = strings.TrimSpace(o.Value)
					o.Label = strings.TrimSpace(o.Label)
					if o.Value == "" || o.Label == "" {
						return fmt.Errorf("题目「%s」的第 %d 个选项不完整", q.Key, oi+1)
					}
					if optSeen[o.Value] {
						return fmt.Errorf("题目「%s」的选项值重复:%s", q.Key, o.Value)
					}
					optSeen[o.Value] = true
				}
			}
			if q.Type == TypeRating {
				if q.Max == 0 {
					q.Max = 5
				}
				if q.Min == 0 {
					q.Min = 1
				}
				if q.Min >= q.Max {
					return fmt.Errorf("题目「%s」的评分区间非法", q.Key)
				}
			}
			if q.Type == TypeImage && q.MaxImages == 0 {
				q.MaxImages = 1
			}
		}
	}
	if total > MaxQuestions {
		return fmt.Errorf("题目过多(最多 %d 道)", MaxQuestions)
	}
	return nil
}

func qt(i int) int { return i + 1 }

func validType(t string) bool {
	for _, x := range QuestionTypes {
		if x == t {
			return true
		}
	}
	return false
}

// FindQuestion returns the question with the given key.
func (d *Definition) FindQuestion(key string) (*Question, bool) {
	for si := range d.Steps {
		for qi := range d.Steps[si].Questions {
			if d.Steps[si].Questions[qi].Key == key {
				return &d.Steps[si].Questions[qi], true
			}
		}
	}
	return nil, false
}

// AllQuestions flattens the definition.
func (d *Definition) AllQuestions() []Question {
	out := make([]Question, 0, 16)
	for _, s := range d.Steps {
		out = append(out, s.Questions...)
	}
	return out
}

// Answers maps a question key to its submitted value. The dynamic type depends
// on the question type: string, []string, float64 or int.
type Answers map[string]any

// ValidateAnswers checks a decoded submission against the definition.
//
// imageIDs is the set of upload IDs that exist for this questionnaire, so an
// answer cannot reference an image that was never uploaded.
func ValidateAnswers(d *Definition, a Answers, imageIDs map[uint]bool) error {
	questions := d.AllQuestions()
	known := make(map[string]bool, len(questions))
	for i := range questions {
		q := &questions[i]
		known[q.Key] = true
		v, present := a[q.Key]

		if q.Required && (!present || isEmptyValue(v)) {
			return fmt.Errorf("题目「%s」为必答项", q.Label)
		}
		if !present || isEmptyValue(v) {
			continue
		}

		switch q.Type {
		case TypeSingle, TypeSelect:
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("题目「%s」的答案类型应为文本", q.Label)
			}
			if !optionExists(q, s) {
				return fmt.Errorf("题目「%s」的答案不在选项中:%s", q.Label, s)
			}
		case TypeMulti:
			list, ok := toStringSlice(v)
			if !ok {
				return fmt.Errorf("题目「%s」的答案类型应为列表", q.Label)
			}
			for _, s := range list {
				if !optionExists(q, s) {
					return fmt.Errorf("题目「%s」的答案不在选项中:%s", q.Label, s)
				}
			}
		case TypeText, TypeTextarea:
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("题目「%s」的答案类型应为文本", q.Label)
			}
			n := utf8.RuneCountInString(s)
			if q.MinLength > 0 && n < q.MinLength {
				return fmt.Errorf("题目「%s」至少需要 %d 个字", q.Label, q.MinLength)
			}
			if q.MaxLength > 0 && n > q.MaxLength {
				return fmt.Errorf("题目「%s」最多 %d 个字", q.Label, q.MaxLength)
			}
		case TypeRating:
			n, ok := toInt(v)
			if !ok {
				return fmt.Errorf("题目「%s」的答案类型应为数字", q.Label)
			}
			if n < q.Min || n > q.Max {
				return fmt.Errorf("题目「%s」的评分需在 %d 到 %d 之间", q.Label, q.Min, q.Max)
			}
		case TypeDate:
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("题目「%s」的答案类型应为日期", q.Label)
			}
			if _, err := time.Parse("2006-01-02", s); err != nil {
				return fmt.Errorf("题目「%s」的日期格式应为 YYYY-MM-DD", q.Label)
			}
		case TypeImage:
			list, ok := toUintSlice(v)
			if !ok {
				return fmt.Errorf("题目「%s」的答案类型应为图片列表", q.Label)
			}
			if q.MaxImages > 0 && len(list) > q.MaxImages {
				return fmt.Errorf("题目「%s」最多上传 %d 张图片", q.Label, q.MaxImages)
			}
			for _, id := range list {
				if !imageIDs[id] {
					return fmt.Errorf("题目「%s」引用了不存在的图片:%d", q.Label, id)
				}
			}
		}
	}

	for k := range a {
		if !known[k] {
			return fmt.Errorf("答案包含未知题目:%s", k)
		}
	}
	return nil
}

func optionExists(q *Question, value string) bool {
	for _, o := range q.Options {
		if o.Value == value {
			return true
		}
	}
	return false
}

func isEmptyValue(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	case []string:
		return len(t) == 0
	case []any:
		return len(t) == 0
	}
	return false
}

func toStringSlice(v any) ([]string, bool) {
	switch t := v.(type) {
	case []string:
		return t, true
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			s, ok := x.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	}
	return nil, false
}

func toUintSlice(v any) ([]uint, bool) {
	list, ok := v.([]any)
	if !ok {
		if t, ok := v.([]uint); ok {
			return t, true
		}
		return nil, false
	}
	out := make([]uint, 0, len(list))
	for _, x := range list {
		n, ok := toInt(x)
		if !ok || n < 0 {
			return nil, false
		}
		out = append(out, uint(n))
	}
	return out, true
}

func toInt(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		// JSON numbers arrive as float64; reject non-integers rather than
		// silently truncating a rating of 3.7 into 3.
		if t != float64(int(t)) {
			return 0, false
		}
		return int(t), true
	case int:
		return t, true
	case int64:
		return int(t), true
	}
	return 0, false
}
