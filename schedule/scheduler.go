package schedule

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ============================================================================
// Course blocks
//
// A CourseBlock groups all height variants of one (format, type, grade-range).
// All classes in a block share a single course layout; only jump heights change
// between them, so they run back-to-back with just a course walk in between.
// ============================================================================

type CourseBlock struct {
	Key     string
	Format  ClassFormat
	Type    ClassType
	Grades  []int
	Classes []*Class
}

func (b *CourseBlock) TotalEntries() int {
	n := 0
	for _, c := range b.Classes {
		n += c.Entries
	}
	return n
}

// ConflictsWith reports whether any class in a conflicts with any class in b
// (shared height with overlapping grades). Type is irrelevant to conflicts.
func (a *CourseBlock) ConflictsWith(b *CourseBlock) bool {
	for _, ca := range a.Classes {
		for _, cb := range b.Classes {
			if ca.ConflictsWith(cb) {
				return true
			}
		}
	}
	return false
}

func courseBlockKey(format ClassFormat, typ ClassType, grades []int) string {
	g := append([]int(nil), grades...)
	sort.Ints(g)
	parts := make([]string, len(g))
	for i, v := range g {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join([]string{string(format), string(typ), strings.Join(parts, ",")}, ":")
}

// groupIntoCourseBlocks partitions G/C classes into course blocks. Open-format
// classes (Anysize/ABC) are returned separately; each becomes its own singleton
// block later, since each open class is its own one-off course.
func groupIntoCourseBlocks(classes []*Class) (blocks []*CourseBlock, open []*Class) {
	blockMap := make(map[string]*CourseBlock)
	var order []string

	for _, cls := range classes {
		if cls.IsOpen() {
			open = append(open, cls)
			continue
		}
		key := courseBlockKey(cls.Format, cls.Type, cls.Grades)
		if _, ok := blockMap[key]; !ok {
			blockMap[key] = &CourseBlock{Key: key, Format: cls.Format, Type: cls.Type, Grades: cls.Grades}
			order = append(order, key)
		}
		blockMap[key].Classes = append(blockMap[key].Classes, cls)
	}
	for _, key := range order {
		blocks = append(blocks, blockMap[key])
	}
	return
}

func singletonBlock(c *Class) *CourseBlock {
	return &CourseBlock{
		Key:     fmt.Sprintf("open:%d", c.Number),
		Format:  c.Format,
		Type:    c.Type,
		Grades:  c.Grades,
		Classes: []*Class{c},
	}
}

// ============================================================================
// Discipline + height helpers
// ============================================================================

const (
	discAgility = iota
	discJumping // jumping + steeplechase share jumping-family equipment
)

// discipline maps a class type to the equipment discipline of its ring.
// Steeplechase lives with jumping (it is never run in an agility ring in
// practice), so it counts as jumping for ring-purity purposes.
func discipline(t ClassType) int {
	if t == TypeAgility {
		return discAgility
	}
	return discJumping
}

func blockDiscipline(b *CourseBlock) int { return discipline(b.Type) }

// standardHeightOrder is the canonical KC height ordering used for snaking.
var standardHeightOrder = []Height{HeightLarge, HeightMedium, HeightSmall, HeightIntermediate}

// sortedBlockClasses returns a block's classes ordered so ring r leads with a
// rotated height, snaking the order across rings/bands.
func sortedBlockClasses(block *CourseBlock, ringIndex int) []*Class {
	n := len(standardHeightOrder)
	posOf := func(h Height) int {
		for k, sh := range standardHeightOrder {
			if sh == h {
				return (k - ringIndex%n + n) % n
			}
		}
		return n // unknown heights (e.g. Anysize) sort last
	}
	out := append([]*Class(nil), block.Classes...)
	sort.SliceStable(out, func(i, j int) bool { return posOf(out[i].Height) < posOf(out[j].Height) })
	return out
}

func gradeStart(grades []int) int {
	if len(grades) == 0 {
		return 99 // open / no-grade classes sort last
	}
	m := grades[0]
	for _, g := range grades {
		if g < m {
			m = g
		}
	}
	return m
}

func equalGrades(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ============================================================================
// Cost weights — the knobs a show manager can tune.
//
// Priority order learned from real ring plans: type-pure rings and tight course
// grouping dominate; balance and clashes are handled largely by good structure,
// with clash-minutes as the fine-grained tie-breaker.
// ============================================================================

type Weights struct {
	MixedRingDiscipline  float64 // per ring that contains both agility and jumping
	DisciplineTransition float64 // per adjacent agility<->jumping switch within a ring
	CourseChange         float64 // per grade/type change between consecutive classes
	BalanceDevPerEntry   float64 // per entry a ring is away from the target
	ClashMinute          float64 // per minute of overlapping clashing classes
	DaySpanMinute        float64 // per minute of spread within a (height, grade) cohort
}

func DefaultWeights() Weights {
	return Weights{
		MixedRingDiscipline:  600,
		DisciplineTransition: 300,
		CourseChange:         30,
		BalanceDevPerEntry:   2.0,
		ClashMinute:          60.0,
		DaySpanMinute:        0.15,
	}
}

const defaultTargetEntriesPerRing = 210

// localSearchRestarts is how many randomised restarts to run in addition to the
// constructive seed. Steepest descent can settle in a local optimum; restarting
// from shuffled layouts and keeping the cheapest result explores wider. The RNG
// is seeded deterministically so the same input always yields the same plan.
const localSearchRestarts = 8

// ============================================================================
// Internal plan representation
// ============================================================================

type ringLayout struct {
	discipline int
	ringIndex  int // global ring index (0-based) — drives height snaking + judges
	blocks     []*CourseBlock
}

func cloneLayouts(in []ringLayout) []ringLayout {
	out := make([]ringLayout, len(in))
	for i := range in {
		out[i] = ringLayout{
			discipline: in[i].discipline,
			ringIndex:  in[i].ringIndex,
			blocks:     append([]*CourseBlock(nil), in[i].blocks...),
		}
	}
	return out
}

func (rl ringLayout) classes() []*Class {
	var out []*Class
	for _, b := range rl.blocks {
		out = append(out, sortedBlockClasses(b, rl.ringIndex)...)
	}
	return out
}

func (rl ringLayout) entries() int {
	n := 0
	for _, b := range rl.blocks {
		n += b.TotalEntries()
	}
	return n
}

// ============================================================================
// Scheduler
// ============================================================================

func Schedule(classes []*Class, opts ScheduleOptions) *RingPlan {
	tc := opts.Timing
	if tc.SecsPerDog == 0 {
		tc = DefaultTiming()
	}
	startTime := opts.StartTime
	if startTime.IsZero() {
		startTime = time.Date(2000, 1, 1, 9, 0, 0, 0, time.UTC)
	}
	weights := DefaultWeights()

	totalEntries := 0
	for _, c := range classes {
		totalEntries += c.Entries
	}

	// --- Build the full block list (course blocks + singleton open blocks) ---
	blocks, openClasses := groupIntoCourseBlocks(classes)
	for _, c := range openClasses {
		blocks = append(blocks, singletonBlock(c))
	}

	// --- Decide ring count and split it between disciplines by entry volume ---
	numRings := opts.NumRings
	if numRings <= 0 {
		numRings = int(math.Round(float64(totalEntries) / defaultTargetEntriesPerRing))
	}
	if opts.MinRings > 0 && numRings < opts.MinRings {
		numRings = opts.MinRings
	}
	if numRings < 1 {
		numRings = 1
	}
	maxAllowed := 12
	if opts.MaxRings > 0 {
		maxAllowed = opts.MaxRings
	}
	if numRings > maxAllowed {
		numRings = maxAllowed
	}

	var agEntries, juEntries int
	for _, b := range blocks {
		if blockDiscipline(b) == discAgility {
			agEntries += b.TotalEntries()
		} else {
			juEntries += b.TotalEntries()
		}
	}

	ringsAg := 0
	if agEntries+juEntries > 0 {
		ringsAg = int(math.Round(float64(numRings) * float64(agEntries) / float64(agEntries+juEntries)))
	}
	if agEntries > 0 && ringsAg < 1 {
		ringsAg = 1
	}
	if juEntries > 0 && ringsAg > numRings-1 {
		ringsAg = numRings - 1
	}
	if juEntries == 0 {
		ringsAg = numRings
	}
	if agEntries == 0 {
		ringsAg = 0
	}
	ringsJu := numRings - ringsAg

	// --- Lay out rings: agility rings first (global indices 0..ringsAg-1),
	//     then jumping rings. ---
	layouts := make([]ringLayout, 0, numRings)
	for i := 0; i < ringsAg; i++ {
		layouts = append(layouts, ringLayout{discipline: discAgility, ringIndex: len(layouts)})
	}
	for i := 0; i < ringsJu; i++ {
		layouts = append(layouts, ringLayout{discipline: discJumping, ringIndex: len(layouts)})
	}
	if len(layouts) == 0 {
		layouts = append(layouts, ringLayout{discipline: discAgility, ringIndex: 0})
	}

	// --- Constructive seed: balance blocks across rings of their discipline,
	//     ordered by grade, then rotate each ring's bands to stagger grades
	//     across concurrent rings. ---
	assignDiscipline := func(disc int) {
		var idxs []int
		for i := range layouts {
			if layouts[i].discipline == disc {
				idxs = append(idxs, i)
			}
		}
		if len(idxs) == 0 {
			// no ring of this discipline (e.g. all-jumping show): fold into ring 0
			idxs = []int{0}
		}
		var pool []*CourseBlock
		for _, b := range blocks {
			if blockDiscipline(b) == disc {
				pool = append(pool, b)
			}
		}
		// Largest blocks first into the least-loaded ring → even entries.
		sort.SliceStable(pool, func(i, j int) bool { return pool[i].TotalEntries() > pool[j].TotalEntries() })
		for _, b := range pool {
			best := idxs[0]
			for _, r := range idxs {
				if layouts[r].entries() < layouts[best].entries() {
					best = r
				}
			}
			layouts[best].blocks = append(layouts[best].blocks, b)
		}
		// Within each ring, order bands by grade then format; then rotate by the
		// ring's position so concurrent rings open on different grade bands.
		for pos, r := range idxs {
			bs := layouts[r].blocks
			sort.SliceStable(bs, func(i, j int) bool {
				if gradeStart(bs[i].Grades) != gradeStart(bs[j].Grades) {
					return gradeStart(bs[i].Grades) < gradeStart(bs[j].Grades)
				}
				return string(bs[i].Format) < string(bs[j].Format)
			})
			if len(bs) > 1 {
				off := pos % len(bs)
				layouts[r].blocks = append(append([]*CourseBlock(nil), bs[off:]...), bs[:off]...)
			}
		}
	}
	assignDiscipline(discAgility)
	assignDiscipline(discJumping)

	// --- Polish with steepest-descent local search plus randomised restarts. ---
	layouts = localSearchWithRestarts(layouts, localSearchRestarts, tc, startTime, weights, totalEntries)

	// --- Materialise rings with clock times. ---
	rings := make([]*Ring, len(layouts))
	for i, rl := range layouts {
		ring := &Ring{Number: i + 1, Name: fmt.Sprintf("Ring %d", i+1)}
		if i < len(opts.Judges) {
			ring.Judge = opts.Judges[i]
		}
		for _, slot := range timeRing(rl.classes(), startTime, tc) {
			ring.Slots = append(ring.Slots, slot)
		}
		rings[i] = ring
	}

	clashes := detectClashes(rings)

	finish := startTime
	for _, r := range rings {
		if n := len(r.Slots); n > 0 && r.Slots[n-1].EndTime.After(finish) {
			finish = r.Slots[n-1].EndTime
		}
	}
	finishStr := ""
	if finish.After(startTime) {
		finishStr = finish.Format("15:04")
	}

	return &RingPlan{
		ShowName: opts.ShowName,
		Date:     opts.Date,
		Rings:    rings,
		Clashes:  clashes,
		Timing:   tc,
		Stats: Stats{
			TotalClasses:    len(classes),
			TotalEntries:    totalEntries,
			NumRings:        len(rings),
			EstimatedFinish: finishStr,
			ClashCount:      len(clashes),
		},
	}
}

// timeRing walks an ordered class list and assigns clock times. Gap logic lives
// in tc.GapMins (first class / height change / course change).
func timeRing(classes []*Class, start time.Time, tc TimingConfig) []*ClassSlot {
	var slots []*ClassSlot
	var prev *Class
	t := start
	for _, c := range classes {
		gap := tc.GapMins(prev, c)
		st := t.Add(time.Duration(gap) * time.Minute)
		en := st.Add(tc.RunDuration(c.Entries))
		slots = append(slots, &ClassSlot{Class: c, StartTime: st, EndTime: en, GapMins: gap})
		t = en
		prev = c
	}
	return slots
}

// ============================================================================
// Cost function
// ============================================================================

func evaluate(layouts []ringLayout, tc TimingConfig, start time.Time, w Weights, totalEntries int) (float64, [][]*ClassSlot) {
	slotsByRing := make([][]*ClassSlot, len(layouts))
	for i, rl := range layouts {
		slotsByRing[i] = timeRing(rl.classes(), start, tc)
	}

	var cost float64

	// Structure costs, per ring.
	target := 0.0
	if len(layouts) > 0 {
		target = float64(totalEntries) / float64(len(layouts))
	}
	for i, rl := range layouts {
		cls := rl.classes()
		seenAg, seenJu := false, false
		for k, c := range cls {
			if discipline(c.Type) == discAgility {
				seenAg = true
			} else {
				seenJu = true
			}
			if k > 0 {
				prev := cls[k-1]
				if c.Type != prev.Type || !equalGrades(c.Grades, prev.Grades) {
					cost += w.CourseChange
				}
				if discipline(c.Type) != discipline(prev.Type) {
					cost += w.DisciplineTransition
				}
			}
		}
		if seenAg && seenJu {
			cost += w.MixedRingDiscipline
		}
		cost += w.BalanceDevPerEntry * math.Abs(float64(rl.entries())-target)
		_ = i
	}

	// Clash cost: overlapping minutes between conflicting classes in different rings.
	for a := 0; a < len(slotsByRing); a++ {
		for b := a + 1; b < len(slotsByRing); b++ {
			for _, sa := range slotsByRing[a] {
				for _, sb := range slotsByRing[b] {
					if !sa.Class.ConflictsWith(sb.Class) {
						continue
					}
					if m := overlapMinutes(sa.StartTime, sa.EndTime, sb.StartTime, sb.EndTime); m > 0 {
						cost += w.ClashMinute * m
					}
				}
			}
		}
	}
	// Day-span cost: for each (height, grade) cohort — the set of classes a
	// single dog of that height and grade could enter — penalise the spread
	// between the cohort's earliest start and latest end. Keeps a competitor's
	// eligible classes from being scattered across the whole day (the "ran first
	// then waited until late afternoon" problem). Open classes are excluded:
	// they never share a dog with G/C classes.
	if w.DaySpanMinute > 0 {
		type span struct{ min, max time.Time }
		cohorts := make(map[[2]int]*span) // key: {heightCode, grade}
		hcode := func(h Height) int {
			for i, sh := range standardHeightOrder {
				if sh == h {
					return i
				}
			}
			return len(standardHeightOrder)
		}
		for _, slots := range slotsByRing {
			for _, s := range slots {
				if s.Class.IsOpen() {
					continue
				}
				for _, g := range s.Class.Grades {
					key := [2]int{hcode(s.Class.Height), g}
					cur, ok := cohorts[key]
					if !ok {
						cohorts[key] = &span{s.StartTime, s.EndTime}
						continue
					}
					if s.StartTime.Before(cur.min) {
						cur.min = s.StartTime
					}
					if s.EndTime.After(cur.max) {
						cur.max = s.EndTime
					}
				}
			}
		}
		for _, sp := range cohorts {
			cost += w.DaySpanMinute * sp.max.Sub(sp.min).Minutes()
		}
	}
	return cost, slotsByRing
}

func overlapMinutes(aStart, aEnd, bStart, bEnd time.Time) float64 {
	start := aStart
	if bStart.After(start) {
		start = bStart
	}
	end := aEnd
	if bEnd.Before(end) {
		end = bEnd
	}
	d := end.Sub(start).Minutes()
	if d < 0 {
		return 0
	}
	return d
}

// ============================================================================
// Local search (steepest descent: relocate / reorder / swap blocks)
// ============================================================================

// localSearchWithRestarts runs steepest descent from the constructive seed and
// from several shuffled starts, keeping the cheapest result.
func localSearchWithRestarts(seed []ringLayout, restarts int, tc TimingConfig, start time.Time, w Weights, totalEntries int) []ringLayout {
	best := localSearch(seed, tc, start, w, totalEntries)
	bestCost, _ := evaluate(best, tc, start, w, totalEntries)

	rng := rand.New(rand.NewSource(1)) // fixed seed → reproducible plans
	for k := 0; k < restarts; k++ {
		cand := localSearch(perturbLayouts(seed, rng), tc, start, w, totalEntries)
		if c, _ := evaluate(cand, tc, start, w, totalEntries); c < bestCost-1e-9 {
			best, bestCost = cand, c
		}
	}
	return best
}

// perturbLayouts produces a randomised starting layout: each block is reassigned
// to a random ring of its own discipline, in random order. Disciplines are
// walked in fixed order so the result depends only on the RNG state.
func perturbLayouts(seed []ringLayout, rng *rand.Rand) []ringLayout {
	out := cloneLayouts(seed)
	for i := range out {
		out[i].blocks = nil
	}
	var ringsByDisc [2][]int
	for i := range out {
		ringsByDisc[out[i].discipline] = append(ringsByDisc[out[i].discipline], i)
	}
	var blocksByDisc [2][]*CourseBlock
	for _, rl := range seed {
		for _, b := range rl.blocks {
			blocksByDisc[rl.discipline] = append(blocksByDisc[rl.discipline], b)
		}
	}
	for disc := 0; disc < 2; disc++ {
		bs := append([]*CourseBlock(nil), blocksByDisc[disc]...)
		rings := ringsByDisc[disc]
		if len(rings) == 0 {
			if len(out) == 0 {
				continue
			}
			rings = []int{0}
		}
		rng.Shuffle(len(bs), func(i, j int) { bs[i], bs[j] = bs[j], bs[i] })
		for k, b := range bs {
			out[rings[k%len(rings)]].blocks = append(out[rings[k%len(rings)]].blocks, b)
		}
	}
	return out
}

func localSearch(layouts []ringLayout, tc TimingConfig, start time.Time, w Weights, totalEntries int) []ringLayout {
	const maxPasses = 60
	current := cloneLayouts(layouts)
	bestCost, _ := evaluate(current, tc, start, w, totalEntries)

	for pass := 0; pass < maxPasses; pass++ {
		improvedThisPass := false
		var bestCand []ringLayout

		consider := func(cand []ringLayout) {
			c, _ := evaluate(cand, tc, start, w, totalEntries)
			if c < bestCost-1e-9 {
				bestCost = c
				bestCand = cand
				improvedThisPass = true
			}
		}

		// Cross-ring relocation: move block (i,p) to (j,q).
		for i := range current {
			for p := range current[i].blocks {
				blk := current[i].blocks[p]
				for j := range current {
					if i == j {
						continue
					}
					for q := 0; q <= len(current[j].blocks); q++ {
						cand := cloneLayouts(current)
						cand[i].blocks = append(cand[i].blocks[:p], cand[i].blocks[p+1:]...)
						cand[j].blocks = insertAt(cand[j].blocks, q, blk)
						consider(cand)
					}
				}
			}
		}

		// Within-ring reorder: move block (i,p) to position q in the same ring.
		for i := range current {
			for p := range current[i].blocks {
				blk := current[i].blocks[p]
				for q := 0; q < len(current[i].blocks); q++ {
					if q == p {
						continue
					}
					cand := cloneLayouts(current)
					b := append(cand[i].blocks[:p], cand[i].blocks[p+1:]...)
					cand[i].blocks = insertAt(b, q, blk)
					consider(cand)
				}
			}
		}

		// Cross-ring swap.
		for i := range current {
			for p := range current[i].blocks {
				for j := i + 1; j < len(current); j++ {
					for q := range current[j].blocks {
						cand := cloneLayouts(current)
						cand[i].blocks[p], cand[j].blocks[q] = cand[j].blocks[q], cand[i].blocks[p]
						consider(cand)
					}
				}
			}
		}

		if !improvedThisPass {
			break
		}
		current = bestCand
	}
	return current
}

func insertAt(s []*CourseBlock, idx int, x *CourseBlock) []*CourseBlock {
	if idx > len(s) {
		idx = len(s)
	}
	out := make([]*CourseBlock, 0, len(s)+1)
	out = append(out, s[:idx]...)
	out = append(out, x)
	out = append(out, s[idx:]...)
	return out
}

// ============================================================================
// Final clash report (unchanged behaviour)
// ============================================================================

func detectClashes(rings []*Ring) []Clash {
	var clashes []Clash
	for i := 0; i < len(rings); i++ {
		for j := i + 1; j < len(rings); j++ {
			for _, sa := range rings[i].Slots {
				for _, sb := range rings[j].Slots {
					if sa.Class.ConflictsWith(sb.Class) &&
						timeOverlap(sa.StartTime, sa.EndTime, sb.StartTime, sb.EndTime) {
						clashes = append(clashes, Clash{
							RingA: rings[i].Number, ClassA: sa.Class.Number, NameA: sa.Class.Name,
							RingB: rings[j].Number, ClassB: sb.Class.Number, NameB: sb.Class.Name,
						})
					}
				}
			}
		}
	}
	return clashes
}
