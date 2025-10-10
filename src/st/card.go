package st

import (
	"encoding/json"
	"fmt"

	"github.com/cloudwindy/xitu/st/ccv3"
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
	return &cardType{
		data: c.Data,
	}, nil
}

type cardType struct {
	data ccv3.CharacterCardData
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

	messages, err := c.applyWorldInfo(charDefs, history)
	if err != nil {
		return nil, err
	}

	return c.toOpenAIMessages(messages), nil
}

func (c *cardType) parseOpenAIMessages(openAIMessages []openai.ChatCompletionMessage) ([]message, error) {
	messages := make([]message, 0, len(openAIMessages))
	for i, openAIMessage := range openAIMessages {
		if openAIMessage.Role == openai.ChatMessageRoleSystem {
			return nil, fmt.Errorf("input messages should not contain 'system' Role")
		}
		if i == len(openAIMessages)-1 && (openAIMessage.Role != openai.ChatMessageRoleUser || openAIMessage.Content == "") {
			return nil, fmt.Errorf("the last message must be a non-empty user message")
		}
		msg, err := parseOpenAIMessage(openAIMessage)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

func (c *cardType) toOpenAIMessages(messages []message) []openai.ChatCompletionMessage {
	openAIMessages := make([]openai.ChatCompletionMessage, 0, len(messages))
	for _, msg := range messages {
		openAIMessages = append(openAIMessages, msg.ToOpenAIMessage())
	}
	return openAIMessages
}

func (c *cardType) buildCharDefMessages() []message {
	charDefs := make([]message, 0)
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

func (c *cardType) pushPrompt(messages *[]message, role roleType, prompt string) {
	*messages = append(*messages, message{
		Role:    role,
		Content: c.evalMacros(prompt),
	})
}
