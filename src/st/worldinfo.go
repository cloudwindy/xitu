package st

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/cloudwindy/xitu/st/ccv3"
	"github.com/rs/zerolog/log"
)

func (c *cardType) applyWorldInfo(charDefs []message, history []message) ([]message, error) {
	wi, err := c.checkWorldInfo(history)
	if err != nil {
		return nil, err
	}
	if wi == nil {
		return append(charDefs, history...), nil
	}
	messages := make([]message, 0, len(charDefs)+len(history)+wi.Len())
	for _, entry := range wi.BeforeCharDefs {
		c.pushPrompt(&messages, entry.Role, entry.Content)
	}
	messages = append(messages, charDefs...)
	for _, entry := range wi.AfterCharDefs {
		c.pushPrompt(&messages, entry.Role, entry.Content)
	}
	return messages, nil
}

func (c *cardType) checkWorldInfo(messages []message) (*worldInfoType, error) {
	if c.data.CharacterBook == nil || len(c.data.CharacterBook.Entries) == 0 {
		return nil, nil
	}

	entries, err := newLorebookEntriesFromCCV3(c.data.CharacterBook.Entries)
	if err != nil {
		return nil, err
	}

	buf := c.buildWorldInfoBuffer(messages)
	activated := make(lorebookEntriesType, 0)

	for _, entry := range entries {
		if entry.activated {
			activated.Push(entry)
			continue
		}

		haystack := buf.Get(entry)
		if haystack == "" {
			continue
		}

		for _, key := range entry.Keys {
			if buf.MatchKeys(haystack, key, entry) {
				entry.activated = true
				break
			}
		}

		if entry.activated {
			activated.Push(entry)
			continue
		}
	}

	if len(activated) == 0 {
		return nil, nil
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
			return nil, fmt.Errorf("invalid Lorebook entry: %v", entry.Position)
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
		case ccv3.RoleUser:
			lorebookEntry.Role = user
		case ccv3.RoleAssistant:
			lorebookEntry.Role = assistant
		case ccv3.RoleSystem:
			lorebookEntry.Role = system
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
func (le *lorebookEntriesType) Activated() lorebookEntriesType {
	activated := make(lorebookEntriesType, 0, len(*le))
	for _, entry := range *le {
		if entry.activated {
			activated = append(activated, entry)
		}
	}
	return activated
}

type worldInfoBufferType struct {
	startDepth int
	buf        strings.Builder
	data       ccv3.CharacterCardData
	messages   []message
}

func (c *cardType) buildWorldInfoBuffer(messages []message) worldInfoBufferType {
	return worldInfoBufferType{
		data:     c.data,
		messages: messages,
	}
}

func (w *worldInfoBufferType) Get(e lorebookEntryType) string {
	if e.Depth <= w.startDepth {
		return ""
	}

	if e.Depth < 0 {
		log.Error().Msg(fmt.Sprintf("invalid entry %v", e))
		return ""
	}

	const delim = "\x01\n"
	defer w.buf.Reset()

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
		w.buf.WriteString(w.data.Extensions.DepthPrompt)
	}
	if e.MatchScenario {
		w.buf.WriteString(delim)
		w.buf.WriteString(w.data.Scenario)
	}
	if e.MatchCreatorNotes {
		w.buf.WriteString(delim)
		w.buf.WriteString(w.data.CreatorNotes)
	}

	return w.buf.String()
}

func (w *worldInfoBufferType) MatchKeys(haystack, needle string, e lorebookEntryType) bool {
	re, err := w.parseRegex(needle)
	if err == nil {
		return re.MatchString(needle)
	}
	log.Debug().Err(err).Str("needle", needle).Msg("Ignoring invalid regex pattern.")

	// Fallback to substring match
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
