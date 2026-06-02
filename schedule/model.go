package schedule

import (
	"fmt"
	"time"
)

// ---- Enumerations ----

type ClassFormat string

const (
	FormatGraded   ClassFormat = "G"
	FormatCombined ClassFormat = "C"
	FormatAnysize  ClassFormat = "Anysize"
	FormatABC      ClassFormat = "ABC"
)

// IsOpen returns true for Anysize and ABC classes.
// These have a completely separate entry pool and never conflict with G/C classes.
func (f ClassFormat) IsOpen() bool {
	return f == FormatAnysize || f == FormatABC
}

type ClassType string

const (
	TypeAgility      ClassType = "Agility"
	TypeJumping      ClassType = "Jumping"
	TypeSteeplechase ClassType = "Steeplechase"
)

type Height string

const (
	HeightLarge        Height = "Large"
	HeightMedium       Height = "Medium"
	HeightSmall        Height = "Small"
	HeightIntermediate Height = "Intermediate"
	HeightAnysize      Height = "Anysize"
)

// ---- Class ----

type Class struct {
	Number  int         `json:"number"`
	Name    string      `json:"name"`
	Format  ClassFormat `json:"format"` // G, C, Anysize, ABC
	Type    ClassType   `json:"type"`
	Grades  []int       `json:"grades"` // empty for open classes
	Height  Height      `json:"height"`
	Entries int         `json:"entries"`
}

func (c *Class) IsOpen() bool { return c.Format.IsOpen() }

// GradeLabel returns a display label like "G1-3" or "C4-5".
func (c *Class) GradeLabel() string {
	if len(c.Grades) == 0 {
		return ""
	}
	lo, hi := c.Grades[0], c.Grades[0]
	for _, g := range c.Grades {
		if g < lo {
			lo = g
		}
		if g > hi {
			hi = g
		}
	}
	if lo == hi {
		return fmt.Sprintf("%s%d", c.Format, lo)
	}
	return fmt.Sprintf("%s%d-%d", c.Format, lo, hi)
}

// ConflictsWith returns true if two classes could be entered by the same dog,
// meaning they cannot run simultaneously without causing a clash.
// Open-format classes (Anysize, ABC) never conflict with anything.
func (a *Class) ConflictsWith(b *Class) bool {
	if a.IsOpen() || b.IsOpen() {
		return false
	}
	if a.Height != b.Height {
		return false
	}
	// Grade set intersection
	for _, ag := range a.Grades {
		for _, bg := range b.Grades {
			if ag == bg {
				return true
			}
		}
	}
	return false
}

// IsSameCourse returns true when consecutive classes in a ring share the same
// course layout — only jump heights change, no rebuild needed.
// This is true when type AND grade range are identical.
func (a *Class) IsSameCourse(b *Class) bool {
	if a.IsOpen() || b.IsOpen() {
		return false
	}
	if a.Type != b.Type {
		return false
	}
	return gradesEqual(a.Grades, b.Grades)
}

func gradesEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[int]int, len(a))
	for _, g := range a {
		counts[g]++
	}
	for _, g := range b {
		counts[g]--
		if counts[g] < 0 {
			return false
		}
	}
	return true
}

// ---- Timing ----

// TimingConfig holds configurable timing constants.
type TimingConfig struct {
	SecsPerDog int // seconds each dog takes in the ring (default 60)
	WalkMins   int // course walk before every class (default 10)
	ChangeMins int // additional time for a course change on top of walk (default 15)
}

func DefaultTiming() TimingConfig {
	return TimingConfig{SecsPerDog: 60, WalkMins: 10, ChangeMins: 15}
}

func (tc TimingConfig) secsPerDog() int {
	if tc.SecsPerDog <= 0 {
		return 60
	}
	return tc.SecsPerDog
}
func (tc TimingConfig) walkMins() int {
	if tc.WalkMins <= 0 {
		return 10
	}
	return tc.WalkMins
}
func (tc TimingConfig) changeMins() int {
	if tc.ChangeMins <= 0 {
		return 15
	}
	return tc.ChangeMins
}

// GapMins returns the gap in minutes before next starts, given what ran before it.
// prev == nil means next is the first class of the day.
func (tc TimingConfig) GapMins(prev, next *Class) int {
	walk := tc.walkMins()
	if prev == nil {
		return walk
	}
	if prev.IsSameCourse(next) {
		return walk // height change only
	}
	return walk + tc.changeMins() // course change
}

// RunDuration returns how long a class takes to run.
func (tc TimingConfig) RunDuration(entries int) time.Duration {
	return time.Duration(entries*tc.secsPerDog()) * time.Second
}

// timeOverlap returns true if [as,ae) and [bs,be) overlap.
func timeOverlap(as, ae, bs, be time.Time) bool {
	return as.Before(be) && bs.Before(ae)
}

// ---- ClassSlot ----

// ClassSlot is a class placed in a ring with computed clock times.
type ClassSlot struct {
	Class     *Class    `json:"class"`
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
	GapMins   int       `json:"gapMins"`
}

func (s *ClassSlot) StartStr() string { return s.StartTime.Format("15:04") }
func (s *ClassSlot) EndStr() string   { return s.EndTime.Format("15:04") }

// ---- Ring ----

type Ring struct {
	Number int          `json:"number"`
	Name   string       `json:"name"`
	Judge  string       `json:"judge"`
	Slots  []*ClassSlot `json:"slots"`
}

func (r *Ring) LastClass() *Class {
	if len(r.Slots) == 0 {
		return nil
	}
	return r.Slots[len(r.Slots)-1].Class
}

func (r *Ring) EndTime() time.Time {
	if len(r.Slots) == 0 {
		return time.Time{}
	}
	return r.Slots[len(r.Slots)-1].EndTime
}

func (r *Ring) TotalEntries() int {
	n := 0
	for _, s := range r.Slots {
		n += s.Class.Entries
	}
	return n
}

// ---- Output types ----

// Clash describes two classes in different rings whose running intervals overlap
// and whose dogs could be entered in both.
type Clash struct {
	RingA  int    `json:"ringA"`
	ClassA int    `json:"classA"`
	NameA  string `json:"nameA"`
	RingB  int    `json:"ringB"`
	ClassB int    `json:"classB"`
	NameB  string `json:"nameB"`
}

type Stats struct {
	TotalClasses    int    `json:"totalClasses"`
	TotalEntries    int    `json:"totalEntries"`
	NumRings        int    `json:"numRings"`
	EstimatedFinish string `json:"estimatedFinish"`
	ClashCount      int    `json:"clashCount"`
}

type RingPlan struct {
	ShowName string   `json:"showName"`
	Date     string   `json:"date"`
	Rings    []*Ring  `json:"rings"`
	Clashes  []Clash  `json:"clashes"`
	Stats    Stats    `json:"stats"`
	Timing   TimingConfig `json:"timing"`
}

// ---- ScheduleOptions ----

type ScheduleOptions struct {
	ShowName         string
	Date             string
	StartTime        time.Time // zero → 09:00
	NumRings         int       // 0 → auto
	MinRings         int       // floor for auto-detection; 0 = no floor
	MaxRings         int       // cap for auto-detection; 0 = default cap (8)
	MaxChangesPerRing int      // max course changes per ring per day; 0 → default 3
	Timing           TimingConfig
	Judges           []string // judge name per ring index
}
