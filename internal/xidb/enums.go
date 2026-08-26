package xidb

import (
	"strings"
	"unicode"
)

// Display names for the small game enums. These mirror scripts/enum/job.lua,
// race.lua, nation.lua and skill.lua in the xiserver repo. They are short and
// frozen, so they are written out rather than generated; the long ones (titles)
// are generated instead, see titles_gen.go.

// JobAbbrev is indexed by xi.job. Index 0 is the "no job" slot.
var JobAbbrev = [...]string{
	"---", "WAR", "MNK", "WHM", "BLM", "RDM", "THF", "PLD", "DRK", "BST",
	"BRD", "RNG", "SAM", "NIN", "DRG", "SMN", "BLU", "COR", "PUP", "DNC",
	"SCH", "GEO", "RUN", "MON",
}

var JobName = [...]string{
	"None", "Warrior", "Monk", "White Mage", "Black Mage", "Red Mage",
	"Thief", "Paladin", "Dark Knight", "Beastmaster", "Bard", "Ranger",
	"Samurai", "Ninja", "Dragoon", "Summoner", "Blue Mage", "Corsair",
	"Puppetmaster", "Dancer", "Scholar", "Geomancer", "Rune Fencer",
	"Monstrosity",
}

// JobColumns are the char_jobs / char_exp column names in job-id order,
// starting at job id 1. Both tables use the same names, which is what lets
// one query shape serve levels and exp alike.
var JobColumns = [...]string{
	"war", "mnk", "whm", "blm", "rdm", "thf", "pld", "drk", "bst", "brd",
	"rng", "sam", "nin", "drg", "smn", "blu", "cor", "pup", "dnc", "sch",
	"geo", "run",
}

func JobShort(id int) string {
	if id >= 0 && id < len(JobAbbrev) {
		return JobAbbrev[id]
	}
	return "???"
}

func JobFull(id int) string {
	if id >= 0 && id < len(JobName) {
		return JobName[id]
	}
	return "Unknown"
}

var raceNames = map[int]string{
	1: "Hume", 2: "Hume", 3: "Elvaan", 4: "Elvaan",
	5: "Tarutaru", 6: "Tarutaru", 7: "Mithra", 8: "Galka",
}

// RaceName folds the eight look values into the five playable races. The
// gendered pairs are the same race, so the sex is reported separately.
func RaceName(look int) string {
	if n, ok := raceNames[look]; ok {
		return n
	}
	return "Unknown"
}

// RaceSex reports "M" or "F" for the gendered races. Mithra are always
// female and Galka always male, so those return the fixed value.
func RaceSex(look int) string {
	switch look {
	case 1, 3, 5, 8:
		return "M"
	case 2, 4, 6, 7:
		return "F"
	}
	return ""
}

var nationNames = [...]string{"San d'Oria", "Bastok", "Windurst", "Beastmen", "Other"}

func NationName(id int) string {
	if id >= 0 && id < len(nationNames) {
		return nationNames[id]
	}
	return "Unknown"
}

// NationSlug is used for the per-nation accent colour in the stylesheet.
var nationSlugs = [...]string{"sandoria", "bastok", "windurst", "beastmen", "other"}

func NationSlug(id int) string {
	if id >= 0 && id < len(nationSlugs) {
		return nationSlugs[id]
	}
	return "other"
}

// SkillGroup buckets a char_skills skillid so the player page can show combat,
// magic and craft skills apart from each other.
type SkillGroup string

const (
	SkillCombat    SkillGroup = "Combat"
	SkillDefensive SkillGroup = "Defensive"
	SkillMagic     SkillGroup = "Magic"
	SkillCraft     SkillGroup = "Craft"
	SkillOther     SkillGroup = "Other"
)

type skillDef struct {
	Name  string
	Group SkillGroup
}

var skills = map[int]skillDef{
	1:  {"Hand-to-Hand", SkillCombat},
	2:  {"Dagger", SkillCombat},
	3:  {"Sword", SkillCombat},
	4:  {"Great Sword", SkillCombat},
	5:  {"Axe", SkillCombat},
	6:  {"Great Axe", SkillCombat},
	7:  {"Scythe", SkillCombat},
	8:  {"Polearm", SkillCombat},
	9:  {"Katana", SkillCombat},
	10: {"Great Katana", SkillCombat},
	11: {"Club", SkillCombat},
	12: {"Staff", SkillCombat},
	22: {"Automaton Melee", SkillCombat},
	23: {"Automaton Ranged", SkillCombat},
	24: {"Automaton Magic", SkillCombat},
	25: {"Archery", SkillCombat},
	26: {"Marksmanship", SkillCombat},
	27: {"Throwing", SkillCombat},
	28: {"Guard", SkillDefensive},
	29: {"Evasion", SkillDefensive},
	30: {"Shield", SkillDefensive},
	31: {"Parrying", SkillDefensive},
	32: {"Divine Magic", SkillMagic},
	33: {"Healing Magic", SkillMagic},
	34: {"Enhancing Magic", SkillMagic},
	35: {"Enfeebling Magic", SkillMagic},
	36: {"Elemental Magic", SkillMagic},
	37: {"Dark Magic", SkillMagic},
	38: {"Summoning Magic", SkillMagic},
	39: {"Ninjutsu", SkillMagic},
	40: {"Singing", SkillMagic},
	41: {"String Instrument", SkillMagic},
	42: {"Wind Instrument", SkillMagic},
	43: {"Blue Magic", SkillMagic},
	44: {"Geomancy", SkillMagic},
	45: {"Handbell", SkillMagic},
	48: {"Fishing", SkillCraft},
	49: {"Woodworking", SkillCraft},
	50: {"Smithing", SkillCraft},
	51: {"Goldsmithing", SkillCraft},
	52: {"Clothcraft", SkillCraft},
	53: {"Leathercraft", SkillCraft},
	54: {"Bonecraft", SkillCraft},
	55: {"Alchemy", SkillCraft},
	56: {"Cooking", SkillCraft},
	57: {"Synergy", SkillCraft},
	58: {"Riding", SkillOther},
	59: {"Digging", SkillOther},
}

func SkillName(id int) (string, bool) {
	if d, ok := skills[id]; ok {
		return d.Name, true
	}
	return "", false
}

func SkillGroupOf(id int) SkillGroup {
	if d, ok := skills[id]; ok {
		return d.Group
	}
	return SkillOther
}

// SkillGroupOrder is the order the player page renders the buckets in.
var SkillGroupOrder = []SkillGroup{SkillCombat, SkillDefensive, SkillMagic, SkillCraft, SkillOther}

// craftRanks maps a craft skill value to its synthesis rank. There are ten
// ranks over the 0-100 skill range, one per ten skill. Craft skill is stored at
// ten times its displayed value, same as combat skill, so the thresholds here
// are in stored units.
var craftRanks = []struct {
	Min  int
	Name string
}{
	{900, "Veteran"},
	{800, "Adept"},
	{700, "Artisan"},
	{600, "Craftsman"},
	{500, "Journeyman"},
	{400, "Apprentice"},
	{300, "Novice"},
	{200, "Initiate"},
	{100, "Recruit"},
	{0, "Amateur"},
}

func CraftRank(value int) string {
	for _, r := range craftRanks {
		if value >= r.Min {
			return r.Name
		}
	}
	return "Amateur"
}

// ZoneDisplay turns a zone_settings.name into something readable.
//
// The server stores zone names with underscores for spaces and with the
// apostrophe dropped, which leaves a lowercase-uppercase seam: Southern_San_dOria,
// RuLude_Gardens, Escha_ZiTah. Every one of the 29 zone names in zone_settings
// that has such a seam wants an apostrophe there, so the rule is applied
// unconditionally.
func ZoneDisplay(raw string) string {
	if raw == "" || raw == "unknown" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(raw) + 4)

	var prev rune
	for _, r := range raw {
		if r == '_' {
			b.WriteRune(' ')
			prev = r
			continue
		}
		if unicode.IsUpper(r) && unicode.IsLower(prev) {
			b.WriteRune('\'')
		}
		b.WriteRune(r)
		prev = r
	}

	return b.String()
}
