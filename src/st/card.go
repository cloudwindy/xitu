package st

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwindy/xitu/st/ccv3"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

// Card 定义一个 SillyTavern 角色卡
type Card interface {
	// GetName 返回角色名称
	GetName() string
	// GetData 返回角色卡的完整数据
	GetData() ccv3.CharacterCardData
	// Apply 将角色卡应用到给定的消息数组
	Apply([]openai.ChatCompletionMessage) ([]openai.ChatCompletionMessage, error)
}

type CardSettings struct {
	UserName       string
	UserPersona    string
	NewMainChat    string
	NewExampleChat string
}

// NewCard 解析并返回一个新的 Card 实例
func NewCard(data []byte, settings ...CardSettings) (Card, error) {
	c := ccv3.CharacterCard{}
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	if c.Spec != "chara_card_v3" || c.SpecVersion != "3.0" {
		return nil, fmt.Errorf("invalid spec or spec_version")
	}
	if c.Data.Name == "" {
		return nil, fmt.Errorf("character name is required")
	}
	card := &cardType{
		data: c.Data,
	}
	if c.Data.CharacterBook != nil && len(c.Data.CharacterBook.Entries) > 0 {
		entries, err := newLorebookEntriesFromCCV3(c.Data.CharacterBook.Entries)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CharacterBook entries: %w", err)
		}
		log.Debug().Int("count", len(c.Data.CharacterBook.Entries)).Msg("CharacterBook entries loaded")
		card.lorebook = entries
	}
	if len(settings) > 0 {
		card.CardSettings = settings[0]
	}
	card.initDefaultSettings()
	return card, nil
}

type cardType struct {
	data     ccv3.CharacterCardData
	lorebook lorebookEntriesType
	CardSettings
}

func (c *cardType) initDefaultSettings() {
	s := &c.CardSettings
	if s.UserName == "" {
		s.UserName = "用户"
	}
	if s.NewMainChat == "" {
		s.NewMainChat = "[Start a new Chat]"
	}
	if s.NewExampleChat == "" {
		s.NewExampleChat = "[Example Chat]"
	}
}

func (c *cardType) GetName() string {
	return c.data.Name
}

func (c *cardType) GetData() ccv3.CharacterCardData {
	return c.data
}

func (c *cardType) Apply(openAIMessages []openai.ChatCompletionMessage) ([]openai.ChatCompletionMessage, error) {
	history, err := c.parseOpenAIMessages(openAIMessages)
	if err != nil {
		return nil, err
	}

	ev := log.Debug().Int("entries", c.lorebook.Len())

	charDefs := c.buildCharDefMessages()
	ev.Int("charDefs", len(charDefs))

	examples := c.buildExampleMessages(c.data.MesExample)
	ev.Int("examples", len(examples))

	messages := make([]messageType, 0, len(charDefs)+len(history))
	wi, err := c.checkWorldInfo(history)
	if err != nil {
		return nil, err
	}
	if wi == nil {
		messages = append(messages, charDefs...)
		messages = append(messages, examples...)
	} else {
		c.applyLorebookEntries(&messages, wi.BeforeCharDefs)
		messages = append(messages, charDefs...)
		c.applyLorebookEntries(&messages, wi.AfterCharDefs)

		beforeEM := c.buildExampleMessages(c.joinLorebookEntries(wi.BeforeExampleMessages))
		afterEM := c.buildExampleMessages(c.joinLorebookEntries(wi.AfterExampleMessages))
		messages = append(messages, beforeEM...)
		messages = append(messages, examples...)
		messages = append(messages, afterEM...)

		ev.Int("activated", wi.ActivatedCount())
	}

	mainChat := c.buildMainChat(history, wi)
	messages = append(messages, mainChat...)

	ev.Int("mainChat", len(mainChat)).
		Int("total", len(messages)).
		Msg("CharacterCard applied")

	return c.toOpenAIMessages(messages), nil
}

func (c *cardType) parseOpenAIMessages(openAIMessages []openai.ChatCompletionMessage) ([]messageType, error) {
	messages := make([]messageType, 0, len(openAIMessages))
	for i, openAIMessage := range openAIMessages {
		if openAIMessage.Role == openai.ChatMessageRoleSystem {
			return nil, fmt.Errorf("input messages should not contain 'system' Role")
		}
		if i == len(openAIMessages)-1 && (openAIMessage.Role != openai.ChatMessageRoleUser || openAIMessage.Content == "") {
			return nil, fmt.Errorf("the last messageType must be a non-empty user messageType")
		}
		msg, err := parseOpenAIMessage(openAIMessage)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

func (c *cardType) toOpenAIMessages(messages []messageType) []openai.ChatCompletionMessage {
	openAIMessages := make([]openai.ChatCompletionMessage, 0, len(messages))
	for _, msg := range messages {
		openAIMessages = append(openAIMessages, msg.ToOpenAIMessage())
	}
	return openAIMessages
}

func (c *cardType) buildCharDefMessages() []messageType {
	messages := make([]messageType, 0)
	c.pushPrompt(&messages, system, c.UserPersona)
	c.pushPrompt(&messages, system, c.data.Description)
	c.pushPrompt(&messages, system, c.data.Personality)
	c.pushPrompt(&messages, system, c.data.Scenario)
	log.Debug().Int("count", len(c.lorebook)).Msg("CharDef built")
	return messages
}

func (c *cardType) buildExampleMessages(mesExample string) []messageType {
	if mesExample == "" {
		return nil
	}
	messages := make([]messageType, 0)
	examples := strings.Split(mesExample, "<START>")
	const delim = "\x01\x02"
	replacer := strings.NewReplacer(
		"{{user}}:", delim,
		"{{char}}:", delim,
	)
	entries := 0
	lines := 0
	for _, example := range examples {
		if c.processPrompt(example) == "" {
			continue
		}
		c.pushPrompt(&messages, system, c.NewExampleChat)
		entries++
		example = replacer.Replace(example)
		for _, line := range strings.Split(example, delim) {
			if c.processPrompt(line) == "" {
				continue
			}
			c.pushPrompt(&messages, system, line)
			lines++
		}
	}
	log.Debug().Int("entries", entries).Int("lines", lines).Msg("ExampleMessages built")
	return messages
}

func (c *cardType) buildMainChat(history []messageType, wi *worldInfoType) []messageType {
	messages := make([]messageType, 0)
	c.pushPrompt(&messages, system, c.NewMainChat)
	entries := lorebookEntriesType{}
	if wi == nil {
		return append(messages, history...)
	}
	for i, msg := range history {
		if i == 0 {
			entries = wi.AtDepth.Depth(len(history), -1)
			if entries.Len() > 0 {
				c.applyLorebookEntries(&messages, entries)
			}
		}
		depth := len(history) - i - 1
		messages = append(messages, msg)
		entries = wi.AtDepth.Depth(depth, depth)
		if entries.Len() > 0 {
			c.applyLorebookEntries(&messages, entries)
		}
	}
	return messages
}

func (c *cardType) pushPrompt(messages *[]messageType, role roleType, prompt string) {
	if prompt != "" {
		*messages = append(*messages, messageType{
			Role:    role,
			Content: c.processPrompt(prompt),
		})
	}
}

func (c *cardType) processPrompt(prompt string) string {
	prompt = strings.Trim(prompt, " \r\n")
	return c.evalMacros(prompt)
}

func (c *cardType) applyLorebookEntries(messages *[]messageType, entries lorebookEntriesType) {
	if len(entries) > 0 {
		for _, role := range entries.Roles() {
			roleEntries := entries.Role(role)
			content := strings.Join(roleEntries.Contents(), "\n")
			c.pushPrompt(messages, role, content)
		}
	}
}

func (c *cardType) joinLorebookEntries(entries lorebookEntriesType) string {
	entries.Sort()
	return strings.Join(entries.Contents(), "\n")
}
