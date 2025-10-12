package st

import (
	"bytes"
	"fmt"
	"math/rand"
	"regexp"
	"sort"
	"strings"

	"github.com/cloudwindy/xitu/st/ccv3"
	"github.com/rs/zerolog/log"
)

func (c *cardType) checkWorldInfo(messages []messageType) (*worldInfoType, error) {
	if len(c.lorebook) == 0 {
		return nil, nil
	}
	lorebook := c.lorebook.Copy()
	lorebook.Sort()

	count := 0
	state := scanStateInitial
	buf := c.buildWorldInfoBuffer(messages)
	activated := make(lorebookEntriesType, 0)
	newEntries := make(lorebookEntriesType, 0)

	for state != scanStateNone {
		count++
		nextState := scanStateNone

		log.Debug().Int("count", count).Str("state", state.String()).Msg("checkWorldInfo iteration start")
		for i, entry := range lorebook {
			if entry.activated {
				continue
			}

			// Not Activated if uses probability and roll fails
			if entry.UseProbability && !roll(entry.Probability) {
				lorebook[i].rollFailed = true
				log.Debug().Str("name", entry.Name).Int("probability", entry.Probability).Msg("Lorebook entry roll failed")
				continue
			}

			// Activated if constant
			if entry.Constant {
				lorebook[i].activated = true
				newEntries.Push(entry)
				log.Debug().Str("name", entry.Name).Msg("Lorebook entry is constant")
				continue
			}

			// Not Activated if delayed until recursion and not in recursion state
			if entry.DelayUntilRecursion && state != scanStateRecursion {
				log.Debug().Str("name", entry.Name).Msg("Lorebook entry delayed until recursion")
				continue
			}

			// Not Activated if excludes recursion and in recursion state
			if entry.ExcludeRecursion && state == scanStateRecursion {
				log.Debug().Str("name", entry.Name).Msg("Lorebook entry excluded in recursion")
				continue
			}

			// Not Activated if no keys to match against
			if len(entry.Keys) == 0 {
				log.Debug().Str("name", entry.Name).Msg("Lorebook entry has no keys to match against")
				continue
			}

			// Not Activated if no text to match against
			if buf.Load(entry) == 0 {
				log.Debug().Str("name", entry.Name).Msg("Lorebook entry cannot match against empty context")
				continue
			}

			// Activated if matches any key
			for _, key := range entry.Keys {
				if buf.Match(key, entry) {
					lorebook[i].activated = true
					newEntries.Push(entry)
					log.Debug().Str("name", entry.Name).Str("key", key).Msg("Lorebook entry matches key")
					break
				}
			}
		}

		if len(newEntries) > 0 {
			activated.Push(newEntries...)
		}
		remaining := lorebook.Len() - activated.Len()
		if len(newEntries.Recursive()) > 0 && remaining > 0 {
			nextState = scanStateRecursion
			buf.ResetRecurse()
			for _, entry := range newEntries.Recursive() {
				buf.WriteRecurse(entry.Content)
			}
		}

		newEntries = newEntries[:0]
		state = nextState
		log.Debug().Int("remaining", remaining).Msg("checkWorldInfo iteration end")
	}

	if dp := c.data.Extensions.DepthPrompt; dp.Prompt != "" {
		role, err := parseOpenAIRole(dp.Role)
		if err != nil {
			return nil, err
		}
		activated.Push(lorebookEntryType{
			Name:      "DepthPrompt",
			Content:   dp.Prompt,
			Role:      role,
			Order:     1024,
			activated: true,
			LorebookEntryExtension: ccv3.LorebookEntryExtension{
				Position: ccv3.LorebookInsertionAtDepth,
				Depth:    dp.Depth,
			},
		})
		log.Debug().Str("role", dp.Role).Int("depth", dp.Depth).Msg("DepthPrompt activated")
	}

	wi := worldInfoType{}
	for _, entry := range activated.Sort() {
		switch entry.Position {
		case ccv3.LorebookInsertionBeforeCharDefs:
			wi.BeforeCharDefs.Unshift(entry)
		case ccv3.LorebookInsertionAfterCharDefs:
			wi.AfterCharDefs.Unshift(entry)
		case ccv3.LorebookInsertionBeforeExampleMessages:
			wi.BeforeExampleMessages.Unshift(entry)
		case ccv3.LorebookInsertionAfterExampleMessages:
			wi.AfterExampleMessages.Unshift(entry)
		case ccv3.LorebookInsertionAtDepth:
			wi.AtDepth.Unshift(entry)
		case ccv3.LorebookInsertionTopOfAuthorsNote:
			wi.TopOfAuthorsNote.Unshift(entry)
		case ccv3.LorebookInsertionBottomOfAuthorsNote:
			wi.BottomOfAuthorsNote.Unshift(entry)
		default:
			return nil, fmt.Errorf("invalid lorebook entry position: %v", entry.Position)
		}
	}
	log.Debug().Int("count", wi.Len()).Msg("WorldInfo built")
	return &wi, nil
}

type scanStateType int

const (
	scanStateNone scanStateType = iota
	scanStateInitial
	scanStateRecursion
)

func (s scanStateType) String() string {
	switch s {
	case scanStateNone:
		return "None"
	case scanStateInitial:
		return "Initial"
	case scanStateRecursion:
		return "Recursion"
	default:
		return "Unknown"
	}
}

type worldInfoType struct {
	BeforeCharDefs        lorebookEntriesType
	AfterCharDefs         lorebookEntriesType
	BeforeExampleMessages lorebookEntriesType
	AfterExampleMessages  lorebookEntriesType
	AtDepth               lorebookEntriesType
	TopOfAuthorsNote      lorebookEntriesType
	BottomOfAuthorsNote   lorebookEntriesType
}

func (l *worldInfoType) Len() int {
	return len(l.BeforeCharDefs) + len(l.AfterCharDefs) +
		len(l.BeforeExampleMessages) + len(l.AfterExampleMessages) +
		len(l.AtDepth) + len(l.TopOfAuthorsNote) + len(l.BottomOfAuthorsNote)
}

type lorebookEntryType struct {
	Name     string
	Keys     []string
	Content  string
	Role     roleType
	Order    int
	Constant bool

	activated  bool
	rollFailed bool
	ccv3.LorebookEntryExtension
}

func newLorebookEntriesFromCCV3(entries []ccv3.LorebookEntry) (lorebookEntriesType, error) {
	lorebookEntries := make(lorebookEntriesType, 0, len(entries))
	for _, entry := range entries {
		if !entry.Enabled {
			continue
		}
		if entry.Extensions.Vectorized {
			log.Warn().Str("name", entry.Name).Msg("Vectorized lorebook entries are not supported and will be ignored")
			continue
		}
		lorebookEntry := lorebookEntryType{
			Name:     entry.Comment,
			Keys:     entry.Keys,
			Content:  entry.Content,
			Order:    entry.InsertionOrder,
			Constant: entry.Constant,
		}
		switch entry.Extensions.Role {
		case ccv3.RoleSystem:
			lorebookEntry.Role = system
		case ccv3.RoleUser:
			lorebookEntry.Role = user
		case ccv3.RoleAssistant:
			lorebookEntry.Role = assistant
		default:
			return nil, fmt.Errorf("unknown role: %v", entry.Extensions.Role)
		}
		lorebookEntry.LorebookEntryExtension = entry.Extensions
		lorebookEntries = append(lorebookEntries, lorebookEntry)
	}
	return lorebookEntries, nil
}

type lorebookEntriesType []lorebookEntryType

func (le *lorebookEntriesType) Len() int                             { return len(*le) }
func (le *lorebookEntriesType) Push(entries ...lorebookEntryType)    { *le = append(*le, entries...) }
func (le *lorebookEntriesType) Unshift(entries ...lorebookEntryType) { *le = append(entries, *le...) }
func (le *lorebookEntriesType) Copy() lorebookEntriesType {
	copied := make(lorebookEntriesType, len(*le))
	copy(copied, *le)
	return copied
}
func (le *lorebookEntriesType) Sort() lorebookEntriesType {
	sorted := make(lorebookEntriesType, len(*le))
	copy(sorted, *le)
	// Sort by Order (descending)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Order > sorted[j].Order
	})
	*le = sorted
	return sorted
}
func (le *lorebookEntriesType) Role(role roleType) lorebookEntriesType {
	filtered := make(lorebookEntriesType, 0, len(*le))
	for _, entry := range *le {
		if entry.Role == role {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}
func (le *lorebookEntriesType) Roles() []roleType {
	roleSet := make(map[roleType]struct{})
	for _, entry := range *le {
		roleSet[entry.Role] = struct{}{}
	}
	roles := make([]roleType, 0, len(roleSet))
	for role := range roleSet {
		roles = append(roles, role)
	}
	return roles
}
func (le *lorebookEntriesType) Contents() []string {
	contents := make([]string, 0, len(*le))
	for _, entry := range *le {
		contents = append(contents, entry.Content)
	}
	return contents
}
func (le *lorebookEntriesType) Depth(min, max int) lorebookEntriesType {
	filtered := make(lorebookEntriesType, 0, len(*le))
	for _, entry := range *le {
		if entry.Depth >= min && (max < 0 || entry.Depth <= max) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}
func (le *lorebookEntriesType) Recursive() lorebookEntriesType {
	filtered := make(lorebookEntriesType, 0, len(*le))
	for _, entry := range *le {
		if !entry.PreventRecursion && !entry.rollFailed {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func (c *cardType) buildWorldInfoBuffer(messages []messageType) worldInfoBufferType {
	w := worldInfoBufferType{
		data: c.data,
	}
	for _, msg := range messages {
		w.WriteDepth(msg.Content)
	}
	return w
}

const worldInfoDelim = "\x01\n"

type worldInfoBufferType struct {
	startDepth     int
	haystackBuffer bytes.Buffer
	depthBuffer    bytes.Buffer
	recurseBuffer  bytes.Buffer
	data           ccv3.CharacterCardData
}

func (w *worldInfoBufferType) Load(e lorebookEntryType) int {
	if e.Depth <= w.startDepth {
		return 0
	}

	if e.Depth < 0 {
		log.Error().Msg(fmt.Sprintf("invalid entry %v", e))
		return 0
	}
	w.haystackBuffer.Reset()

	_, _ = w.depthBuffer.WriteTo(&w.haystackBuffer)

	if e.MatchPersonaDescription {
		w.haystackBuffer.WriteString(worldInfoDelim)
		w.haystackBuffer.WriteString(DefaultUserPersona)
	}
	if e.MatchCharacterDescription {
		w.haystackBuffer.WriteString(worldInfoDelim)
		w.haystackBuffer.WriteString(w.data.Description)
	}
	if e.MatchCharacterPersonality {
		w.haystackBuffer.WriteString(worldInfoDelim)
		w.haystackBuffer.WriteString(w.data.Personality)
	}
	if e.MatchCharacterDepthPrompt {
		w.haystackBuffer.WriteString(worldInfoDelim)
		w.haystackBuffer.WriteString(w.data.Extensions.DepthPrompt.Prompt)
	}
	if e.MatchScenario {
		w.haystackBuffer.WriteString(worldInfoDelim)
		w.haystackBuffer.WriteString(w.data.Scenario)
	}
	if e.MatchCreatorNotes {
		w.haystackBuffer.WriteString(worldInfoDelim)
		w.haystackBuffer.WriteString(w.data.CreatorNotes)
	}
	if w.recurseBuffer.Len() > 0 {
		w.haystackBuffer.WriteString(worldInfoDelim)
		_, _ = w.recurseBuffer.WriteTo(&w.haystackBuffer)
	}

	return w.haystackBuffer.Len()
}

func (w *worldInfoBufferType) Len() int {
	return w.haystackBuffer.Len()
}

func (w *worldInfoBufferType) Match(needle string, e lorebookEntryType) bool {
	re, err := w.parseRegex(needle)
	if err == nil {
		return re.MatchString(needle)
	}
	log.Debug().Err(err).Str("needle", needle).Msg("Ignoring invalid regex pattern.")

	// Fallback to substring match
	haystack := w.haystackBuffer.String()
	if e.CaseSensitive == nil || !*e.CaseSensitive {
		haystack = strings.ToLower(haystack)
		needle = strings.ToLower(needle)
	}

	if e.MatchWholeWords == nil || *e.MatchWholeWords {
		keywords := reKeywords.FindAllString(needle, -1)
		if len(keywords) > 1 {
			// All keywords must match
			return strings.Contains(haystack, needle)
		}
		// Match whole word
		reWholeWord := regexp.MustCompile(`(?:^|\\W)(` + regexp.QuoteMeta(needle) + `)(?:$|\\W)`)
		return reWholeWord.MatchString(haystack)
	}
	return strings.Contains(haystack, needle)
}

func (w *worldInfoBufferType) WriteDepth(str string) {
	w.depthBuffer.WriteString(worldInfoDelim)
	w.depthBuffer.WriteString(str)
}

func (w *worldInfoBufferType) WriteRecurse(str string) {
	w.depthBuffer.WriteString(worldInfoDelim)
	w.recurseBuffer.WriteString(str)
}

func (w *worldInfoBufferType) ResetRecurse() {
	w.recurseBuffer.Reset()
}

func (w *worldInfoBufferType) Reset() {
	w.haystackBuffer.Reset()
	w.depthBuffer.Reset()
	w.recurseBuffer.Reset()
}

var reKeywords = regexp.MustCompile(`\s+`)
var reRegex = regexp.MustCompile(`^/([\w\W]+?)/([gimsuy]*)$`)
var reUnescapedSlash = regexp.MustCompile(`(^|[^\\])/`)

func (w *worldInfoBufferType) parseRegex(pattern string) (*regexp.Regexp, error) {
	matches := reRegex.FindStringSubmatch(pattern)
	if len(matches) != 3 {
		return nil, fmt.Errorf("invalid regex pattern: %s", pattern)
	}
	re := matches[1]
	flags := matches[2]
	if flags != "" {
		flagStr := ""
		if strings.Contains(flags, "i") {
			flagStr = flagStr + "i"
		}
		if strings.Contains(flags, "m") {
			flagStr = flagStr + "m"
		}
		if strings.Contains(flags, "s") {
			flagStr = flagStr + "s"
		}
		re = "(?" + flagStr + ")" + re
	}
	if reUnescapedSlash.MatchString(re) {
		return nil, fmt.Errorf("unescaped slash in regex pattern: %s", pattern)
	}
	re = strings.ReplaceAll(re, "\\/", "/")
	return regexp.Compile(re)
}

func roll(probability int) bool {
	if probability <= 0 {
		return false
	}
	if probability >= 100 {
		return true
	}
	return rand.Intn(100)+1 <= probability
}
