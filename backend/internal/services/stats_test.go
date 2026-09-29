package services

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"qrforqa/internal/codec"
	"qrforqa/internal/config"
	"qrforqa/internal/database"
	"qrforqa/internal/models"
	"qrforqa/internal/schema"
	"qrforqa/internal/vault"
)

const testMasterKey = "services-test-master-key-0123456789"

func newTestService(t *testing.T) *Service {
	t.Helper()

	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"), false)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	v, err := vault.New(testMasterKey)
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	cfg := &config.Config{
		MasterKey:            testMasterKey,
		Layout:               codec.LayoutASCII,
		TokenTTL:             time.Hour,
		AdminUser:            "admin",
		AdminPassword:        "test-password",
		MaxUploadCipherBytes: 1 << 20,
	}
	return New(db, v, cfg)
}

// seededQuestionnaire creates a published questionnaire with one multi-choice
// question and one single-choice question, returning it along with the key the
// browser would be handed.
func seededQuestionnaire(t *testing.T, svc *Service) (*models.Questionnaire, string, []schema.Question) {
	t.Helper()

	def := schema.Definition{
		Title: "统计测试",
		Steps: []schema.Step{{
			ID:    "s1",
			Title: "第一步",
			Questions: []schema.Question{
				{
					Key: "topics", Type: schema.TypeMulti, Label: "关注方向", Required: false,
					Options: []schema.Option{
						{Value: "perf", Label: "性能"},
						{Value: "sec", Label: "安全"},
						{Value: "ux", Label: "体验"},
					},
				},
				{
					Key: "role", Type: schema.TypeSingle, Label: "角色", Required: true,
					Options: []schema.Option{
						{Value: "dev", Label: "开发"},
						{Value: "ops", Label: "运维"},
					},
				},
				{Key: "score", Type: schema.TypeRating, Label: "评分", Required: true, Min: 1, Max: 5},
			},
		}},
	}

	q, err := svc.Create(QuestionnaireInput{Status: models.StatusPublished, Definition: def})
	if err != nil {
		t.Fatalf("create questionnaire: %v", err)
	}
	return q, svc.Vault().DerivedKey(q.ID), def.AllQuestions()
}

func submit(t *testing.T, svc *Service, q *models.Questionnaire, key string, answers map[string]any) *models.Response {
	t.Helper()
	cipher, err := svc.Vault().EncodeJSON(q.ID, codec.LayoutASCII, answers)
	if err != nil {
		t.Fatalf("encode answers: %v", err)
	}
	resp, err := svc.Submit(q.Slug, cipher, "ascii", 1000, "", "test-agent")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	return resp
}

// Regression test: a multi-choice answer counts once per RESPONDENT, not once
// per selection. Summing selections made Skipped = total - Answered negative as
// soon as anyone ticked more than one box.
func TestStatsCountsRespondentsNotSelections(t *testing.T) {
	svc := newTestService(t)
	q, key, _ := seededQuestionnaire(t, svc)

	submit(t, svc, q, key, map[string]any{
		"topics": []string{"perf", "sec", "ux"}, // three selections, one respondent
		"role":   "dev",
		"score":  5,
	})
	submit(t, svc, q, key, map[string]any{
		"topics": []string{"perf"},
		"role":   "ops",
		"score":  3,
	})
	// A respondent who skipped the optional multi-choice question entirely.
	submit(t, svc, q, key, map[string]any{"role": "dev", "score": 4})

	stats, err := svc.Stats(q)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.Total != 3 {
		t.Fatalf("total: got %d want 3", stats.Total)
	}

	byKey := map[string]QuestionStat{}
	for _, qs := range stats.Questions {
		byKey[qs.Key] = qs
	}

	topics := byKey["topics"]
	if topics.Answered != 2 {
		t.Errorf("multi answered: got %d want 2 respondents", topics.Answered)
	}
	if topics.Skipped != 1 {
		t.Errorf("multi skipped: got %d want 1", topics.Skipped)
	}
	if topics.Skipped < 0 {
		t.Error("skipped went negative, which is the bug this test exists for")
	}

	// The distribution still counts selections, which is what a percentage of
	// picks should reflect.
	var perf OptionStat
	for _, o := range topics.Options {
		if o.Value == "perf" {
			perf = o
		}
	}
	if perf.Count != 2 {
		t.Errorf("perf selections: got %d want 2", perf.Count)
	}

	role := byKey["role"]
	if role.Answered != 3 || role.Skipped != 0 {
		t.Errorf("single choice: answered %d skipped %d, want 3 and 0", role.Answered, role.Skipped)
	}

	score := byKey["score"]
	if score.Answered != 3 {
		t.Errorf("rating answered: got %d want 3", score.Answered)
	}
	if score.Average != 4 {
		t.Errorf("rating average: got %v want 4", score.Average)
	}
}

func TestAnsweredValue(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want bool
	}{
		{"nil", nil, false},
		{"empty string", "", false},
		{"whitespace only", "   ", false},
		{"text", "hello", true},
		{"empty list", []any{}, false},
		{"list", []any{"a"}, true},
		{"number", float64(0), true}, // a rating of 0 is still an answer
	}
	for _, c := range cases {
		if got := answeredValue(c.in); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

// The definition is stored as ciphertext and must never appear in plaintext.
func TestDefinitionIsStoredEncoded(t *testing.T) {
	svc := newTestService(t)
	q, _, _ := seededQuestionnaire(t, svc)

	if q.SchemaCipher == "" {
		t.Fatal("schema ciphertext is empty")
	}
	for _, secret := range []string{"统计测试", "关注方向", "性能"} {
		if strings.Contains(q.SchemaCipher, secret) {
			t.Errorf("stored definition leaks plaintext %q", secret)
		}
	}

	// And it must still decode back to exactly what was stored.
	def, err := svc.Definition(q)
	if err != nil {
		t.Fatalf("decode definition: %v", err)
	}
	if def.Title != "统计测试" || len(def.Steps) != 1 || len(def.Steps[0].Questions) != 3 {
		t.Fatalf("definition did not round-trip: %+v", def)
	}
}
