package st

import (
	"encoding/json"
	"fmt"

	"github.com/cloudwindy/xitu/st/ccv3"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

var (
	DefaultUserName    = "用户"
	DefaultUserPersona = ""
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

// NewCard 解析并返回一个新的 Card 实例
func NewCard(data []byte) (Card, error) {
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
	return card, nil
}

type cardType struct {
	data     ccv3.CharacterCardData
	lorebook lorebookEntriesType
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

	charDefs := c.buildCharDefMessages()
	wi, err := c.checkWorldInfo(history)
	if err != nil {
		return nil, err
	}
	if wi == nil {
		return c.toOpenAIMessages(append(charDefs, history...)), nil
	}

	messages := make([]messageType, 0, len(charDefs)+len(history)+wi.Len())

	c.applyLorebookEntries(&messages, wi.BeforeCharDefs)
	messages = append(messages, charDefs...)
	c.applyLorebookEntries(&messages, wi.AfterCharDefs)

	entries := lorebookEntriesType{}
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
	charDefs := make([]messageType, 0)
	if DefaultUserPersona != "" {
		c.pushPrompt(&charDefs, system, DefaultUserPersona)
	}
	if c.data.Description != "" {
		c.pushPrompt(&charDefs, system, c.data.Description)
	}
	if c.data.Personality != "" {
		c.pushPrompt(&charDefs, system, c.data.Personality)
	}
	if c.data.Scenario != "" {
		c.pushPrompt(&charDefs, system, c.data.Scenario)
	}
	return charDefs
}

func (c *cardType) pushPrompt(messages *[]messageType, role roleType, prompt string) {
	*messages = append(*messages, messageType{
		Role:    role,
		Content: c.evalMacros(prompt),
	})
}
