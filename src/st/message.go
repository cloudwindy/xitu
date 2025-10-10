package st

import (
	"fmt"

	"github.com/sashabaranov/go-openai"
)

type message struct {
	Role    roleType
	Content string
}

func parseOpenAIMessage(msg openai.ChatCompletionMessage) (message, error) {
	role, err := parseOpenAIRole(msg.Role)
	if err != nil {
		return message{}, err
	}
	return message{
		Role:    role,
		Content: msg.Content,
	}, nil
}

func (msg message) ToOpenAIMessage() openai.ChatCompletionMessage {
	return openai.ChatCompletionMessage{
		Role:    msg.Role.ToOpenAIRole(),
		Content: msg.Content,
	}
}

type roleType int

const (
	user roleType = iota
	assistant
	system
)

func parseOpenAIRole(role string) (roleType, error) {
	switch role {
	case openai.ChatMessageRoleSystem:
		return system, nil
	case openai.ChatMessageRoleUser:
		return user, nil
	case openai.ChatMessageRoleAssistant:
		return assistant, nil
	default:
		return 0, fmt.Errorf("invalid OpenAI Role: %s", role)
	}
}

func (role roleType) ToOpenAIRole() string {
	switch role {
	case system:
		return openai.ChatMessageRoleSystem
	case user:
		return openai.ChatMessageRoleUser
	case assistant:
		return openai.ChatMessageRoleAssistant
	default:
		panic(fmt.Errorf("invalid Role: %v", role))
	}
}
