package st

import (
	"regexp"
)

func (c *cardType) evalMacros(prompt string) string {
	staticMacros := newCaseInsensitiveReplacer(
		"{{newline}}", "\n",
		"{{noop}}", "",
		"{{user}}", c.UserName,
		"<USER>", c.UserName,
		"{{char}}", c.data.Name,
		"<BOT>", c.data.Name,
		"{{description}}", c.data.Description,
		"{{scenario}}", c.data.Scenario,
		"{{personality}}", c.data.Personality,
		"{{persona}}", c.UserPersona,
		"{{mesExamplesRaw}}", c.data.MesExample,
	)
	return staticMacros.Replace(prompt)
}

type caseInsensitiveReplacer struct {
	toReplaces   []*regexp.Regexp
	replaceWiths []string
}

func newCaseInsensitiveReplacer(oldnew ...string) *caseInsensitiveReplacer {
	if len(oldnew)%2 != 0 {
		panic("newCaseInsensitiveReplacer: odd number of arguments")
	}

	toReplaces := make([]*regexp.Regexp, 0, len(oldnew)/2)
	replaceWiths := make([]string, 0, len(oldnew)/2)
	for i := 0; i < len(oldnew); i += 2 {
		if oldnew[i] == "" {
			panic("newCaseInsensitiveReplacer: empty old string")
		}
		re := regexp.MustCompile("(?i)" + regexp.QuoteMeta(oldnew[i]))
		toReplaces = append(toReplaces, re)
		replaceWiths = append(replaceWiths, oldnew[i+1])
	}
	return &caseInsensitiveReplacer{
		toReplaces:   toReplaces,
		replaceWiths: replaceWiths,
	}
}

func (cir *caseInsensitiveReplacer) Replace(str string) string {
	for i, re := range cir.toReplaces {
		str = re.ReplaceAllString(str, cir.replaceWiths[i])
	}
	return str
}
