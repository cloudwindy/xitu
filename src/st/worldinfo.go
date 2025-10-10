package st

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cloudwindy/xitu/st/ccv3"
	"github.com/rs/zerolog/log"
)

func (c *cardType) checkWorldInfo(messages []message) (*lorebookType, error) {
	if c.data.CharacterBook == nil || len(c.data.CharacterBook.Entries) == 0 {
		return nil, nil
	}
	lb := lorebookType{}
	wi := c.buildWorldInfoBuffer(messages)
	for _, ccv3Entry := range c.data.CharacterBook.Entries {
		// TODO
		if wi.get(ccv3Entry) {
			entry := lorebookEntryType{}
			switch ccv3Entry.Extensions.Position {
			case ccv3.LorebookInsertionBeforeCharDefs:
				lb.BeforeCharDefs = append(lb.BeforeCharDefs, entry)
			case ccv3.LorebookInsertionAfterCharDefs:
				lb.AfterCharDefs = append(lb.AfterCharDefs, entry)
			case ccv3.LorebookInsertionBeforeExampleMessages:
				lb.BeforeExampleMessages = append(lb.BeforeExampleMessages, entry)
			case ccv3.LorebookInsertionAfterExampleMessages:
				lb.AfterExampleMessages = append(lb.AfterExampleMessages, entry)
			case ccv3.LorebookInsertionAtDepth:
				lb.AtDepth = append(lb.AtDepth, entry)
			case ccv3.LorebookInsertionTopOfAuthorsNote:
				lb.TopOfAuthorsNote = append(lb.TopOfAuthorsNote, entry)
			case ccv3.LorebookInsertionBottomOfAuthorsNote:
				lb.BottomOfAuthorsNote = append(lb.BottomOfAuthorsNote, entry)
			default:
				return nil, fmt.Errorf("invalid Lorebook entry: %v", ccv3Entry.Extensions.Position)
			}
		}
	}
	return &lb, nil
}

type scanStateType int

const (
	scanStateNone = iota
	scanStateInitial
	scanStateRecursion
	scanStateMinActivations
)

type lorebookType struct {
	BeforeCharDefs        []lorebookEntryType
	AfterCharDefs         []lorebookEntryType
	BeforeExampleMessages []lorebookEntryType
	AfterExampleMessages  []lorebookEntryType
	AtDepth               []lorebookEntryType
	TopOfAuthorsNote      []lorebookEntryType
	BottomOfAuthorsNote   []lorebookEntryType
}

type lorebookEntryType struct {
	Content string
	Role    roleType

	activated bool
	ccv3.LorebookEntryExtension
}

func newLorebookEntryFromCCV3(entry ccv3.LorebookEntry) (*lorebookEntryType, error) {
	le := lorebookEntryType{
		Content: entry.Content,
	}
	switch entry.Extensions.Role {
	case ccv3.RoleUser:
		le.Role = user
	case ccv3.RoleAssistant:
		le.Role = assistant
	case ccv3.RoleSystem:
		le.Role = system
	default:
		return nil, fmt.Errorf("unknown role: %v", entry.Extensions.Role)
	}
	le.LorebookEntryExtension = entry.Extensions
	return &le, nil
}

type worldInfoBufferType struct {
	data     ccv3.CharacterCardV3Data
	messages []message
}

func (c *cardType) buildWorldInfoBuffer(messages []message) worldInfoBufferType {
	return worldInfoBufferType{
		data:     c.data,
		messages: messages,
	}
}

func (w *worldInfoBufferType) get(e lorebookEntryType, scanState scanStateType) string {
	if e.Depth < 0 {
		log.Error().Msg(fmt.Sprintf("invalid entry %v", e))
	}
	if e.MatchCharacterDescription {
		w.data.
	}
}

func (w *worldInfoBufferType) matchKeys(haystack, needle string, e lorebookEntryType) bool {
	re, err := w.parseRegex(needle)
	if err == nil {
		return re.MatchString(needle)
	}
	log.Debug().Err(err).Str("needle", needle).Msg("Ignoring invalid regex pattern.")

	// Fallback to substring match
	haystack = w.transformString(haystack, e)
	needle = w.transformString(needle, e)

	if e.MatchWholeWords {
		// Unsupported
		log.Debug().Str("needle", needle).Msg("Ignoring unsupported whole-word match.")
	}
	return strings.Contains(haystack, needle)
}

func (w *worldInfoBufferType) transformString(str string, e lorebookEntryType) string {
	if e.CaseSensitive {
		return strings.ToLower(str)
	}
	return str
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
