# Ring Planner — Domain Glossary

## Show

A dog agility competition event. Contains multiple **Rings** running concurrently throughout the day.

---

## Ring

A physical area where classes are run sequentially throughout the day. A ring handles any mix of heights and grades — it is **not** dedicated to a single height. Multiple classes share a ring by running one after another.

Each ring has an assigned **judge** (a named person). The judge is recorded per ring on the ring plan and in the printed output.

In practice a ring runs a single **discipline** all day — see *Scheduling Principles → Type-pure rings*.

---

## Class

A single competitive event defined by:
- **Height** (e.g. Large, Medium, Small, Intermediate, Anysize)
- **Grade range** (e.g. G1–3, C3–5)
- **Type** (Agility, Jumping, Steeplechase)
- **Entries** — number of dogs competing

A class has exactly one height. A dog has exactly one height and competes only in classes at their height — except Anysize (see below).

### Class format prefixes

**G (Graded):** Standard KC graded class. Multiple winners — one per grade within the range. A G1–3 class has three grade winners.

**C (Combined):** KC combined class. One overall winner across all eligible grades. Same dog eligibility as the equivalent G class — a Grade 3 Large dog can enter both G3–5 Large Agility and C1–3 Large Jumping. G and C classes therefore **can clash** with each other.

**Anysize:** A completely separate competition. Dogs entered in Anysize **may not enter any G or C class on the same show day**. Anysize classes therefore **never clash** with G or C classes, and can always run concurrently with them without conflict.

**ABC (Anything But Collie):** Open to all dogs (no grade range, no height restriction). Treated identically to Anysize for scheduling purposes — separate entry pool, never clashes with G or C classes.

---

## Grade

A skill level assigned to a dog (1–7 in UK KC agility). A class specifies an **eligible grade range** — any dog whose grade falls within that range may enter. A Grade 2 dog may enter a "Grade 1–3" class, for example.

---

## Height

A jump-height category assigned permanently to a dog (e.g. Large, Medium, Small, Intermediate). A dog competes **only** in classes at their own height. Two classes at different heights can **never** be entered by the same dog.

---

## Clash

Two classes **clash** when they could be entered by the same dog. The conditions are:

1. **Same height**, AND
2. **Overlapping grade ranges** (at least one grade value appears in both eligible sets)

Classes with different heights never clash, regardless of grades. Classes with the same height but non-overlapping grades also never clash and **may run simultaneously** in different rings. Common grade ranges include G1–2, G1–3, G3–4, G3–5, G4–5, and G6–7. These partially overlap (e.g. G1–3 and G3–5 share grade 3; G3–4 and G4–5 share grade 4), so clash detection must use **set intersection** on the eligible grade sets — not bucket equality or simple range comparison.

A dog may enter **4–5 classes per show day** across any combination of types (Agility, Jumping, Steeplechase) and grade ranges they qualify for. This means two Agility classes with overlapping grades at the same height **can** be entered by the same dog and therefore can clash — type identity alone does not prevent a clash.

### Same-ring co-location (key scheduling fact)

Two classes that clash can still be placed in the **same ring**. Because classes in a ring run strictly one after another, their running intervals can never overlap, so no clash is possible. **Co-locating clashing classes in one ring is the primary way to remove a clash.** Clashes only ever arise **between** rings. (A scheduler must therefore never treat "same height + overlapping grades in the same ring" as a violation — that is the opposite of correct.)

---

## Course

The physical obstacle layout in a ring for a given class. A course is shared across all height variants of the same grade range and type — e.g. "Grade 1–3 Agility" is one course, run at Large height, then Medium, then Small, etc. Only jump heights change between runs; the course itself is unchanged. Heights within the same course may run in **any order** and do **not** all have to be in the same ring.

A new course must be built when the **grade range** or **class type** changes. A type change (e.g. Agility → Jumping) at the same grade range counts as a course change even though the grade range is unchanged.

---

## Course Walk

A 10-minute period before every class during which handlers walk the course without their dogs. Applies to **every** class — including height changes on the same course. The walk before the **first class** of the day occurs before the show start time, so the first class begins exactly at the configured start time (no gap is added in the schedule).

---

## Course Change

Occurs when consecutive classes in the same ring differ in **grade range** or **class type**. Requires 15 minutes to rebuild the course, **plus** the standard 10-minute course walk = **25 minutes total** between those two classes.

A height change on the same course (same grade range, same type) incurs only the 10-minute course walk — no course change time.

---

## Run Time

Each dog takes **60 seconds** in the ring. A class with *n* entries takes *n* minutes to run.

---

## Schedule Duration (for a single class in a ring)

`duration = preceding_gap + entries × 60s`

Where `preceding_gap` is:
- **10 min** if the previous class in this ring was the same grade range **and** same type (height change only)
- **25 min** if the previous class had a different grade range **or** different type (course change: 15 min rebuild + 10 min walk)
- **0 min** for the first class of the day — the course walk happens *before* the configured show start time, so the first class begins exactly at the start time

---

## Ring Plan (output)

The primary output of the scheduler. Displayed as an interactive editor in the UI and exportable as a print-ready document.

**Interactive editor requirements:**
- Classes displayed as draggable cards in a per-ring column layout
- Show managers can drag cards to reorder within a ring or move to a different ring
- Clock times **recalculate in real time** as cards are moved
- Clashes **highlighted in real time** — any class whose running interval overlaps a clashing class in another ring is visually flagged
- Moves that create clashes are **allowed** (not blocked) — clashes are flagged as warnings, not hard errors. Some clashes are unavoidable in practice and the manager decides whether to accept them.
- A "Generate PDF / Print" action produces the final document

**Printed document requirements:**
- One column per ring
- Each class shows: class number, name, grade range, height, type, entry count, estimated start time
- Clash-free (the manager resolves all warnings before printing)

---

## Clash Detection (time-based)

Two classes clash in a schedule if:
1. They satisfy the **Clash** definition above (same height, overlapping grades), AND
2. Their **running intervals overlap in clock time** (not merely occupy the same position index)

The positional model (same row index = concurrent) is insufficient because classes vary significantly in size. A 5-entry class finishes in 5 minutes while a 150-entry class runs for 150 minutes; a fast ring can advance to a clashing class while a slow ring is still mid-run.

---

## Scheduling Principles (learned from real ring plans)

Derived from real published ring plans for shows at the same venue run by **different organisers** (Kelluki, Wyvern). The patterns are consistent across organisers, so the scheduler encodes them as its objective. These describe how a good plan is *structured*; low clash counts emerge from the structure rather than from chasing clashes directly.

### Type-pure rings (dominant principle)

A ring runs a single **discipline** all day: an **Agility ring** or a **Jumping ring**. This minimises equipment changes (agility needs contact equipment that jumping does not). **Steeplechase runs in a jumping ring** — it shares jumping-family equipment and is never placed in an agility ring. Mixing agility and jumping in one ring is a last-resort concession, used only when entries don't divide cleanly into whole rings.

### Ring allocation by entry volume

Total ring count targets ~200–210 entries per ring. Those rings are then split between agility and jumping **in proportion to each discipline's total entries** — e.g. ~2 agility + 3 jumping for 5 rings; a clean 3+3 or 4+4 for 6–8 rings. Agility rings are numbered first. Balance is measured by **entries, not class count**.

### Grade-band staggering (the partition rule)

Within a discipline, concurrent rings are ordered so that at any moment they are running **grade-disjoint** bands. With N rings of one discipline, each time-phase should roughly **partition** the grade spectrum across those rings (e.g. ring A runs grades {1,2}, ring B {3,4}, ring C {5,6,7}). When this holds, clashes within that discipline disappear by construction; the only residue is the unavoidable remainder from uneven band counts or sizes. This staggering — not scattering classes apart — is how organisers reach near-zero clashes.

### Course grouping and height snaking

Within a ring, all height variants of one course (same grade range + type + format) run **consecutively**, so only course walks (not rebuilds) occur between them. Consecutive bands snake the height order (Sml→Lge, then Lge→Sml) so the shared boundary height sits adjacent across the change.

### Graded vs Combined as separate sections

Graded and Combined of the same discipline are typically placed in **different rings** (e.g. a Graded Agility ring and a Combined Agility ring) so their grade bands can be staggered against each other. They are **not** merged into a single course, even though their eligibility overlaps. They still clash by the rules above, so the staggering is what keeps them clash-free.

### Open classes as type-tagged filler

Anysize and ABC classes never clash, so they act as flexible ballast: place each in a ring of its **own discipline** (Anysize Agility → an agility ring; Anysize / ABC Jumping → a jumping ring) wherever a ring is light. They are **not** all crammed at the end of the day.

### Balance vs clashes is a deliberate trade-off

A perfectly even entry split and zero clashes can conflict — evening out two heavy/light rings sometimes reintroduces a clash. Organisers genuinely differ here: some accept a couple of clashes for tighter balance; others keep rings strictly clean and tolerate uneven totals (real plans range from ~175 to ~300 per ring). The scheduler exposes this as **tunable cost weights** rather than hard-coding one policy. Clash avoidance is weighted to outrank course changes by default.

### Competitor day-span (avoid stranding)

A plan can be completely clash-free and still be a bad day for a competitor. A dog has one height and one grade and typically enters 4–5 classes; if those eligible classes are scattered from first thing in the morning to late afternoon, that handler waits around all day. This is **not** a clash (a clash is two eligible classes overlapping in time and is impossible for one dog to attend); it is the opposite failure — eligible classes spread too far apart.

The scheduler penalises, for each **(height, grade) cohort**, the spread between the cohort's earliest start and latest end across the whole schedule. Minimising this interleaves heights and grades through the day so no group is run first-then-abandoned. It pulls against clash avoidance (compressing a cohort's window pushes its same-height classes into the same time band, which is when clashes appear), so it is weighted as a **tie-breaker among clash-free plans only** — the clash weight is set high enough that day-span can never buy a clash. Open classes are excluded (an Anysize dog cannot also be in a G/C class).

---

## Non-standard classes

Real entry data contains classes that don't fit the standard (format, type, single grade range, single height) shape. Rule of thumb: anything **without a normal grade range** is treated as non-clashing, **type-tagged filler** (like Anysize). Never hard-code grade bands — read them from the data and detect clashes by **integer grade-set intersection**, since banding varies by show (e.g. Steeplechase appears as C1–3 at one show, C1–4 at another).

Observed cases:
- **Split-height classes** (e.g. `53a` Lge/Int, `53b` Sml/Med): one class run as two height-group sessions, each with its own running order.
- **Pairs classes** (e.g. Wessex Wyvern Pairs Steeplechase): scored as pairs; scheduled like a normal class of their type.
- **Sponsor / rosette-named classes** (e.g. Norton Rosettes Anysize): treated as Anysize / open.
- **Anysize Agility**: Anysize exists in both agility *and* jumping forms — route each to a ring of the matching discipline.
