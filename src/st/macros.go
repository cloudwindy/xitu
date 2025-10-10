package st

import "strings"

func (c *cardType) evalMacros(prompt string) string {
	staticMacros := strings.NewReplacer(
		"{{newline}}", "\n",
		"{{noop}}", "",
		"{{user}}", DefaultUserName,
		"<USER>", DefaultUserName,
		"{{char}}", c.data.Name,
		"<BOT>", c.data.Name,
		"{{description}}", c.data.Description,
		"{{scenario}}", c.data.Scenario,
		"{{personality}}", c.data.Personality,
		"{{persona}}", DefaultUserPersona,
		"{{mesExamplesRaw}}", c.data.MesExample,
	)
	return staticMacros.Replace(prompt)
}
