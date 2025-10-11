package st

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/cloudwindy/xitu/st/ccv3"
	"github.com/rs/zerolog/log"
)

func (c *cardType) applyLorebookEntries(messages *[]messageType, entries lorebookEntriesType) {
	if len(entries) > 0 {
		for _, role := range entries.Roles() {
			roleEntries := entries.Role(role)
			content := strings.Join(roleEntries.Contents(), "\n")
			c.pushPrompt(messages, role, content)
		}
	}
}

func (c *cardType) checkWorldInfo(messages []messageType) (*worldInfoType, error) {
	if len(c.lorebook) == 0 {
		return nil, nil
	}

	buf := c.buildWorldInfoBuffer(messages)
	activated := make(lorebookEntriesType, 0)
	lorebook := c.lorebook.Copy()

	for _, entry := range lorebook.Sort() {
		if entry.activated {
			activated.Push(entry)
			continue
		}

		if buf.Write(entry) == 0 {
			continue
		}

		for _, key := range entry.Keys {
			if buf.Match(key, entry) {
				entry.activated = true
				break
			}
		}

		if entry.activated {
			activated.Push(entry)
			continue
		}
	}

	if dp := c.data.Extensions.DepthPrompt; dp.Prompt != "" {
		role, err := parseOpenAIRole(dp.Role)
		if err != nil {
			return nil, err
		}
		activated.Push(lorebookEntryType{
			Content:   dp.Prompt,
			Role:      role,
			Order:     1024,
			activated: true,
			LorebookEntryExtension: ccv3.LorebookEntryExtension{
				Position: ccv3.LorebookInsertionAtDepth,
				Depth:    dp.Depth,
			},
		})
	}

	wi := worldInfoType{}
	for _, entry := range activated {
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
	return &wi, nil
}

type scanStateType int

const (
	scanStateNone = iota
	scanStateInitial
	scanStateRecursion
	scanStateMinActivations
)

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
	Keys    []string
	Content string
	Role    roleType
	Order   int

	activated bool
	ccv3.LorebookEntryExtension
}

func newLorebookEntriesFromCCV3(entries []ccv3.LorebookEntry) (lorebookEntriesType, error) {
	lorebookEntries := make(lorebookEntriesType, 0, len(entries))
	for _, entry := range entries {
		if !entry.Enabled {
			continue
		}
		lorebookEntry := lorebookEntryType{
			Keys:      entry.Keys,
			Content:   entry.Content,
			Order:     entry.InsertionOrder,
			activated: entry.Constant,
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

type worldInfoBufferType struct {
	startDepth int
	buf        strings.Builder
	data       ccv3.CharacterCardData
	messages   []messageType
}

func (c *cardType) buildWorldInfoBuffer(messages []messageType) worldInfoBufferType {
	return worldInfoBufferType{
		data:     c.data,
		messages: messages,
	}
}

func (w *worldInfoBufferType) Write(e lorebookEntryType) int {
	if e.Depth <= w.startDepth {
		return 0
	}

	if e.Depth < 0 {
		log.Error().Msg(fmt.Sprintf("invalid entry %v", e))
		return 0
	}

	const delim = "\x01\n"
	w.buf.Reset()

	for i, msg := range w.messages {
		if i != 0 {
			w.buf.WriteString(delim)
		}
		w.buf.WriteString(msg.Content)
	}
	if e.MatchPersonaDescription {
		w.buf.WriteString(delim)
		w.buf.WriteString(DefaultUserPersona)
	}
	if e.MatchCharacterDescription {
		w.buf.WriteString(delim)
		w.buf.WriteString(w.data.Description)
	}
	if e.MatchCharacterPersonality {
		w.buf.WriteString(delim)
		w.buf.WriteString(w.data.Personality)
	}
	if e.MatchCharacterDepthPrompt {
		w.buf.WriteString(delim)
		w.buf.WriteString(w.data.Extensions.DepthPrompt.Prompt)
	}
	if e.MatchScenario {
		w.buf.WriteString(delim)
		w.buf.WriteString(w.data.Scenario)
	}
	if e.MatchCreatorNotes {
		w.buf.WriteString(delim)
		w.buf.WriteString(w.data.CreatorNotes)
	}

	return w.buf.Len()
}

func (w *worldInfoBufferType) Match(needle string, e lorebookEntryType) bool {
	re, err := w.parseRegex(needle)
	if err == nil {
		return re.MatchString(needle)
	}
	log.Debug().Err(err).Str("needle", needle).Msg("Ignoring invalid regex pattern.")

	// Fallback to substring match
	haystack := w.buf.String()
	if !e.CaseSensitive {
		haystack = strings.ToLower(haystack)
		needle = strings.ToLower(needle)
	}

	if e.MatchWholeWords {
		// Unsupported
		log.Debug().Str("needle", needle).Msg("Ignoring unsupported whole-word match.")
	}
	return strings.Contains(haystack, needle)
}

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
