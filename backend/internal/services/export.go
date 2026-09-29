package services

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	"qrforqa/internal/models"
	"qrforqa/internal/schema"
)

// ExportFormat is a supported download format.
type ExportFormat string

const (
	FormatCSV  ExportFormat = "csv"
	FormatXLSX ExportFormat = "xlsx"
	FormatJSON ExportFormat = "json"
)

// ParseExportFormat validates a ?format= value.
func ParseExportFormat(s string) (ExportFormat, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "csv":
		return FormatCSV, nil
	case "xlsx", "excel":
		return FormatXLSX, nil
	case "json":
		return FormatJSON, nil
	default:
		return "", fmt.Errorf("unsupported export format %q", s)
	}
}

// Export renders every response of a questionnaire as DECODED content.
//
// This is the administrator-facing side of the requirement: the database holds
// sudoku ciphertext, and what leaves the export endpoint is the real answers.
func (s *Service) Export(q *models.Questionnaire, format ExportFormat) (data []byte, contentType, filename string, err error) {
	def, err := s.Definition(q)
	if err != nil {
		return nil, "", "", err
	}
	responses, err := s.AllResponses(q)
	if err != nil {
		return nil, "", "", err
	}
	base := fmt.Sprintf("%s-%s", sanitizeFilename(def.Title), q.Slug)

	switch format {
	case FormatJSON:
		buf, err := exportJSON(def, responses)
		return buf, "application/json; charset=utf-8", base + ".json", err
	case FormatXLSX:
		buf, err := s.exportXLSX(def, responses)
		return buf, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", base + ".xlsx", err
	default:
		buf, err := exportCSV(def, responses)
		return buf, "text/csv; charset=utf-8", base + ".csv", err
	}
}

func sanitizeFilename(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "questionnaire"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' ||
			r == '<' || r == '>' || r == '|' || r == '\n' || r == '\r' || r == '\t':
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if len(out) > 60 {
		out = strings.ToValidUTF8(out[:60], "")
	}
	if out == "" {
		return "questionnaire"
	}
	return out
}

// exportRow is one response flattened into display strings.
type exportRow struct {
	Index     int
	CreatedAt string
	DurationS string
	Cells     []string
	Images    []ImageView
}

func buildRows(def *schema.Definition, responses []ResponseView) ([]exportRow, []schema.Question) {
	questions := def.AllQuestions()
	rows := make([]exportRow, 0, len(responses))
	for i := range responses {
		r := &responses[i]
		row := exportRow{
			Index:     r.Index,
			CreatedAt: r.CreatedAt.Format("2006-01-02 15:04:05"),
			DurationS: fmt.Sprintf("%.1f", float64(r.DurationMs)/1000),
			Images:    r.Images,
		}
		for _, q := range questions {
			row.Cells = append(row.Cells, formatAnswer(q, r.Answers[q.Key]))
		}
		rows = append(rows, row)
	}
	return rows, questions
}

func formatAnswer(q schema.Question, v any) string {
	if v == nil {
		return ""
	}
	switch q.Type {
	case schema.TypeSingle, schema.TypeSelect:
		s, _ := v.(string)
		return optionLabel(q, s)
	case schema.TypeMulti:
		list, ok := v.([]any)
		if !ok {
			return ""
		}
		parts := make([]string, 0, len(list))
		for _, x := range list {
			if s, ok := x.(string); ok {
				parts = append(parts, optionLabel(q, s))
			}
		}
		return strings.Join(parts, "；")
	case schema.TypeRating:
		if f, ok := v.(float64); ok {
			return itoa(int(f))
		}
		return ""
	case schema.TypeText, schema.TypeTextarea, schema.TypeDate:
		s, _ := v.(string)
		return s
	case schema.TypeImage:
		list, ok := v.([]any)
		if !ok {
			return ""
		}
		parts := make([]string, 0, len(list))
		for _, x := range list {
			if f, ok := x.(float64); ok {
				parts = append(parts, fmt.Sprintf("图片#%d", int(f)))
			}
		}
		return strings.Join(parts, " ")
	}
	return fmt.Sprint(v)
}

func optionLabel(q schema.Question, value string) string {
	for _, o := range q.Options {
		if o.Value == value {
			return o.Label
		}
	}
	return value
}

func exportCSV(def *schema.Definition, responses []ResponseView) ([]byte, error) {
	rows, questions := buildRows(def, responses)

	var buf bytes.Buffer
	// Excel on Windows needs the UTF-8 BOM to read Chinese correctly; without
	// it the file opens as mojibake. Written as an escape because a literal BOM
	// byte in Go source is a compile error.
	buf.WriteString("\ufeff")
	w := csv.NewWriter(&buf)

	header := []string{"序号", "提交时间", "用时(秒)"}
	for _, q := range questions {
		header = append(header, q.Label)
	}
	header = append(header, "图片数")
	if err := w.Write(header); err != nil {
		return nil, err
	}

	for _, row := range rows {
		rec := []string{itoa(row.Index), row.CreatedAt, row.DurationS}
		rec = append(rec, row.Cells...)
		rec = append(rec, itoa(len(row.Images)))
		if err := w.Write(rec); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func exportJSON(def *schema.Definition, responses []ResponseView) ([]byte, error) {
	questions := def.AllQuestions()

	type questionOut struct {
		Key    string          `json:"key"`
		Type   string          `json:"type"`
		Label  string          `json:"label"`
		Option []schema.Option `json:"options,omitempty"`
	}
	type responseOut struct {
		Index     int            `json:"index"`
		CreatedAt string         `json:"createdAt"`
		DurationS float64        `json:"durationSeconds"`
		Answers   map[string]any `json:"answers"`
		Images    []ImageView    `json:"images"`
	}

	out := struct {
		Title         string        `json:"title"`
		Description   string        `json:"description,omitempty"`
		Slug          string        `json:"slug,omitempty"`
		ExportedAt    string        `json:"exportedAt"`
		ResponseCount int           `json:"responseCount"`
		Questions     []questionOut `json:"questions"`
		Responses     []responseOut `json:"responses"`
	}{
		Title: def.Title, Description: def.Description,
		ResponseCount: len(responses),
		Questions:     make([]questionOut, 0, len(questions)),
		Responses:     make([]responseOut, 0, len(responses)),
	}
	for _, q := range questions {
		out.Questions = append(out.Questions, questionOut{Key: q.Key, Type: q.Type, Label: q.Label, Option: q.Options})
	}
	for i := range responses {
		r := &responses[i]
		out.Responses = append(out.Responses, responseOut{
			Index: r.Index, CreatedAt: r.CreatedAt.Format("2006-01-02 15:04:05"),
			DurationS: float64(r.DurationMs) / 1000,
			Answers:   r.Answers, Images: r.Images,
		})
	}
	return json.MarshalIndent(out, "", "  ")
}

// maxEmbeddedImages bounds picture embedding. Beyond this the workbook becomes
// slow to open for no real benefit, so image columns degrade to a reference.
const maxEmbeddedImages = 60

func (s *Service) exportXLSX(def *schema.Definition, responses []ResponseView) ([]byte, error) {
	rows, questions := buildRows(def, responses)

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	const sheet = "答卷"
	defaultSheet := f.GetSheetName(0)
	if err := f.SetSheetName(defaultSheet, sheet); err != nil {
		return nil, err
	}

	header := []string{"序号", "提交时间", "用时(秒)"}
	for _, q := range questions {
		header = append(header, q.Label)
	}
	header = append(header, "图片数")

	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"3730A3"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
	})
	if err != nil {
		return nil, err
	}
	bodyStyle, err := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "top", WrapText: true},
	})
	if err != nil {
		return nil, err
	}

	for i, h := range header {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return nil, err
		}
	}
	lastCol, _ := excelize.CoordinatesToCellName(len(header), 1)
	if err := f.SetCellStyle(sheet, "A1", lastCol, headerStyle); err != nil {
		return nil, err
	}

	for ri, row := range rows {
		rec := []any{row.Index, row.CreatedAt, row.DurationS}
		for _, c := range row.Cells {
			rec = append(rec, c)
		}
		rec = append(rec, len(row.Images))
		for ci, v := range rec {
			cell, _ := excelize.CoordinatesToCellName(ci+1, ri+2)
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				return nil, err
			}
		}
	}
	if len(rows) > 0 {
		end, _ := excelize.CoordinatesToCellName(len(header), len(rows)+1)
		if err := f.SetCellStyle(sheet, "A2", end, bodyStyle); err != nil {
			return nil, err
		}
	}
	if err := f.SetColWidth(sheet, "A", "A", 6); err != nil {
		return nil, err
	}
	if err := f.SetColWidth(sheet, "B", "B", 20); err != nil {
		return nil, err
	}
	if err := f.SetColWidth(sheet, "C", "C", 10); err != nil {
		return nil, err
	}
	if len(header) > 3 {
		start, _ := excelize.ColumnNumberToName(4)
		end, _ := excelize.ColumnNumberToName(len(header))
		if err := f.SetColWidth(sheet, start, end, 24); err != nil {
			return nil, err
		}
	}
	if err := f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return nil, err
	}

	// A second sheet carries the actual pictures, so the export contains the
	// real content rather than opaque IDs.
	images := make([]ImageView, 0, 16)
	for i := range responses {
		images = append(images, responses[i].Images...)
	}
	if len(images) > 0 {
		const imgSheet = "图片"
		if _, err := f.NewSheet(imgSheet); err != nil {
			return nil, err
		}
		_ = f.SetCellValue(imgSheet, "A1", "答卷序号")
		_ = f.SetCellValue(imgSheet, "B1", "题目")
		_ = f.SetCellValue(imgSheet, "C1", "图片")
		_ = f.SetCellValue(imgSheet, "D1", "说明")
		if err := f.SetCellStyle(imgSheet, "A1", "D1", headerStyle); err != nil {
			return nil, err
		}
		_ = f.SetColWidth(imgSheet, "A", "A", 12)
		_ = f.SetColWidth(imgSheet, "B", "B", 20)
		_ = f.SetColWidth(imgSheet, "C", "C", 40)
		_ = f.SetColWidth(imgSheet, "D", "D", 40)

		embed := len(images) <= maxEmbeddedImages
		for i, img := range images {
			rowNum := i + 2
			_ = f.SetCellValue(imgSheet, fmt.Sprintf("A%d", rowNum), i+1)
			_ = f.SetCellValue(imgSheet, fmt.Sprintf("B%d", rowNum), img.FieldKey)

			caption := fmt.Sprintf("%s · %dx%d · %.0f KB",
				strings.TrimPrefix(img.Mime, "image/"), img.Width, img.Height, float64(img.SizeBytes)/1024)
			if !embed {
				caption = "未内嵌(图片过多),请从管理界面查看 · " + caption
				_ = f.SetCellValue(imgSheet, fmt.Sprintf("D%d", rowNum), caption)
				continue
			}

			raw, mime, err := s.ImageBytes(img.ID)
			if err != nil {
				_ = f.SetCellValue(imgSheet, fmt.Sprintf("D%d", rowNum), "解码失败:"+err.Error())
				continue
			}
			ext := mimeExt(mime)

			// Scale the picture down to a column-friendly width, preserving the
			// aspect ratio, and set the row height to match so the image is not
			// clipped by the row below it.
			w, h := float64(img.Width), float64(img.Height)
			if w <= 0 || h <= 0 {
				w, h = 400, 300
			}
			const targetW = 260.0
			scale := 1.0
			if w > targetW {
				scale = targetW / w
			}
			if scale < 0.05 {
				scale = 0.05
			}
			displayH := h * scale

			err = f.AddPictureFromBytes(imgSheet, fmt.Sprintf("C%d", rowNum), &excelize.Picture{
				Extension: ext,
				File:      raw,
				Format: &excelize.GraphicOptions{
					ScaleX: scale, ScaleY: scale,
					LockAspectRatio: true,
					Positioning:     "oneCell",
					AltText:         caption,
				},
				InsertType: excelize.PictureInsertTypePlaceOverCells,
			})
			_ = f.SetCellValue(imgSheet, fmt.Sprintf("D%d", rowNum), caption)
			if err != nil {
				continue
			}
			// Excel row heights are in points (1px at 96dpi = 0.75pt).
			_ = f.SetRowHeight(imgSheet, rowNum, displayH*0.75)
		}
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// mimeExt returns an extension INCLUDING the leading dot, which is what
// excelize's supportedImageTypes map is keyed on.
func mimeExt(mime string) string {
	switch strings.ToLower(mime) {
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/bmp":
		return ".bmp"
	case "image/tiff":
		return ".tiff"
	default:
		// WebP and anything unrecognised fall back to JPEG encoding, which
		// Excel accepts; the picture may not render as intended in that case,
		// so it is labelled in the caption column.
		return ".jpeg"
	}
}
