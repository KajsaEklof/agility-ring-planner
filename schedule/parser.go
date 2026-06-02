package schedule

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
)

// ParseCSV parses a CSV string into a slice of Classes.
// Required column: entries (or count / dogs / num_entries).
// Optional columns: class_no, class_name, format, type, grades, height.
// Missing optional values are inferred from the class_name when possible.
func ParseCSV(data string) ([]*Class, error) {
	r := csv.NewReader(strings.NewReader(strings.TrimSpace(data)))
	r.TrimLeadingSpace = true
	r.Comment = '#'

	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading CSV: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("CSV must have a header row and at least one data row")
	}

	// Build header → column index map (lower-cased, trimmed)
	hdr := make(map[string]int, len(records[0]))
	for i, h := range records[0] {
		hdr[strings.ToLower(strings.TrimSpace(h))] = i
	}

	col := func(names ...string) int {
		for _, n := range names {
			if i, ok := hdr[n]; ok {
				return i
			}
		}
		return -1
	}

	numCol     := col("class_no", "class no", "classno", "number", "no", "#", "id", "class_number", "class")
	nameCol    := col("class_name", "name", "description", "classname", "class_desc")
	formatCol  := col("format", "class_format", "prefix", "fmt")
	typeCol    := col("type", "class_type", "classtype")
	gradesCol  := col("grades", "grade", "grade_range", "graderange")
	heightCol  := col("height", "ht", "size", "height_cat")
	entriesCol := col("entries", "entry", "entries_count", "count", "dogs", "num_entries", "no_entries")

	if entriesCol < 0 {
		return nil, fmt.Errorf("CSV is missing an 'entries' column (also tried: count, dogs, num_entries)")
	}

	get := func(row []string, i int) string {
		if i < 0 || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	var classes []*Class
	for rowIdx, row := range records[1:] {
		if isEmptyRow(row) {
			continue
		}

		cls := &Class{}

		// Class number
		if numCol >= 0 {
			if n, err2 := strconv.Atoi(get(row, numCol)); err2 == nil {
				cls.Number = n
			}
		}
		if cls.Number == 0 {
			cls.Number = rowIdx + 1
		}

		// Name
		cls.Name = get(row, nameCol)

		// Entries (required)
		entriesStr := get(row, entriesCol)
		n, err2 := strconv.Atoi(entriesStr)
		if err2 != nil || n < 0 {
			return nil, fmt.Errorf("row %d (class %d): invalid entries value %q", rowIdx+2, cls.Number, entriesStr)
		}
		cls.Entries = n

		// Format
		fmtStr := get(row, formatCol)
		cls.Format = inferFormat(fmtStr, cls.Name)

		// Type
		typeStr := get(row, typeCol)
		if typeStr == "" {
			typeStr = extractTypeFromName(cls.Name)
		}
		cls.Type = parseClassType(typeStr)

		// Grades (not applicable for open classes)
		if !cls.Format.IsOpen() {
			gradesStr := get(row, gradesCol)
			if gradesStr == "" {
				gradesStr = extractGradesFromName(cls.Name)
			}
			cls.Grades = parseGrades(gradesStr)
		}

		// Height
		heightStr := get(row, heightCol)
		if heightStr == "" {
			heightStr = extractHeightFromName(cls.Name)
		}
		cls.Height = parseHeight(heightStr)

		// If still no name, generate one
		if cls.Name == "" {
			cls.Name = buildName(cls)
		}

		classes = append(classes, cls)
	}

	return classes, nil
}

// ---- Helpers ----

func isEmptyRow(row []string) bool {
	for _, v := range row {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}

func inferFormat(explicit, name string) ClassFormat {
	s := strings.TrimSpace(explicit)
	switch strings.ToUpper(s) {
	case "G", "GRADED", "GRADE":
		return FormatGraded
	case "C", "COMBINED", "COMB":
		return FormatCombined
	case "ANYSIZE", "ANY SIZE", "ANY":
		return FormatAnysize
	case "ABC":
		return FormatABC
	}
	// Infer from name
	lower := strings.ToLower(name)
	if strings.Contains(lower, "anysize") || strings.Contains(lower, "any size") {
		return FormatAnysize
	}
	if strings.Contains(lower, "abc") {
		return FormatABC
	}
	// Look for leading G/C before a digit in any word
	for _, word := range strings.Fields(name) {
		if len(word) < 2 {
			continue
		}
		prefix := string(word[0])
		if (prefix == "G" || prefix == "C") && isDigit(word[1]) {
			if prefix == "C" {
				return FormatCombined
			}
			return FormatGraded
		}
	}
	return FormatGraded // default
}

func parseClassType(s string) ClassType {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "a", "ag", "agility", "agil":
		return TypeAgility
	case "j", "jp", "jmp", "jumping", "jump":
		return TypeJumping
	case "s", "sc", "steeplechase", "steeple":
		return TypeSteeplechase
	}
	return TypeJumping
}

func parseHeight(s string) Height {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case s == "" || s == "anysize" || s == "any" || s == "any size":
		return HeightAnysize
	case strings.HasPrefix(s, "l"):
		return HeightLarge
	case strings.HasPrefix(s, "m"):
		return HeightMedium
	case strings.HasPrefix(s, "s"):
		return HeightSmall
	case strings.HasPrefix(s, "i"):
		return HeightIntermediate
	}
	return HeightAnysize
}

// parseGrades parses strings like "1-3", "G1-3", "C4-5", "7", "1,2,3".
func parseGrades(s string) []int {
	s = strings.TrimSpace(s)
	// Strip format prefix
	if len(s) > 0 && (s[0] == 'G' || s[0] == 'g' || s[0] == 'C' || s[0] == 'c') {
		s = s[1:]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	if idx := strings.Index(s, "-"); idx >= 0 {
		lo, err1 := strconv.Atoi(strings.TrimSpace(s[:idx]))
		hi, err2 := strconv.Atoi(strings.TrimSpace(s[idx+1:]))
		if err1 == nil && err2 == nil && lo <= hi {
			grades := make([]int, 0, hi-lo+1)
			for g := lo; g <= hi; g++ {
				grades = append(grades, g)
			}
			return grades
		}
	}

	var grades []int
	for _, part := range strings.Split(s, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
			grades = append(grades, n)
		}
	}
	return grades
}

func extractTypeFromName(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "agility") || strings.Contains(lower, " ag ") || strings.HasSuffix(lower, " ag"):
		return "agility"
	case strings.Contains(lower, "jumping") || strings.Contains(lower, " jp") || strings.Contains(lower, "jump"):
		return "jumping"
	case strings.Contains(lower, "steeplechase") || strings.Contains(lower, " sc"):
		return "steeplechase"
	}
	return ""
}

func extractGradesFromName(name string) string {
	for _, word := range strings.Fields(name) {
		if len(word) < 2 {
			continue
		}
		start := 0
		if word[0] == 'G' || word[0] == 'C' || word[0] == 'g' || word[0] == 'c' {
			start = 1
		}
		rest := word[start:]
		if looksLikeGradeRange(rest) {
			return word // return including prefix so inferFormat can see G/C
		}
	}
	return ""
}

func extractHeightFromName(name string) string {
	lower := strings.ToLower(name)
	for _, pair := range []struct{ token, height string }{
		{"large", "large"}, {" lge", "large"}, {"lge ", "large"}, {"lge\t", "large"},
		{"medium", "medium"}, {" med", "medium"}, {"med ", "medium"},
		{"small", "small"}, {" sml", "small"}, {"sml ", "small"},
		{"intermediate", "intermediate"}, {" int", "intermediate"}, {"int ", "intermediate"},
		{"anysize", "anysize"},
	} {
		if strings.Contains(lower, pair.token) {
			return pair.height
		}
	}
	return ""
}

func looksLikeGradeRange(s string) bool {
	if s == "" {
		return false
	}
	for _, ch := range s {
		if ch != '-' && (ch < '0' || ch > '9') {
			return false
		}
	}
	return true
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func buildName(c *Class) string {
	if c.GradeLabel() != "" {
		return fmt.Sprintf("%s %s %s", c.Height, c.Type, c.GradeLabel())
	}
	return fmt.Sprintf("%s %s", c.Height, c.Type)
}
