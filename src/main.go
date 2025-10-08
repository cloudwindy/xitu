package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/cloudwindy/xitu/ccv3"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

type ChatRequest struct {
	CharacterID string                         `json:"characterId" binding:"required"`
	History     []openai.ChatCompletionMessage `json:"history"`
}

func setupLogger() {
	if gin.Mode() == gin.DebugMode {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	}

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if gin.Mode() == gin.DebugMode {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	log.Logger = log.With().Timestamp().Logger()
}

// GinLogger 是一个 Gin 中间件，用于记录每个请求
func GinLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		stop := time.Now()
		latency := stop.Sub(start)
		if raw != "" {
			path = path + "?" + raw
		}

		var logEvent *zerolog.Event
		if c.Writer.Status() >= http.StatusInternalServerError {
			logEvent = log.Error()
		} else if c.Writer.Status() >= http.StatusBadRequest {
			logEvent = log.Warn()
		} else {
			logEvent = log.Info()
		}

		logEvent.
			Str("method", c.Request.Method).
			Str("path", path).
			Int("status", c.Writer.Status()).
			Str("ip", c.ClientIP()).
			Dur("latency", latency).
			Str("user_agent", c.Request.UserAgent()).
			Msg("Request processed")
	}
}

func loadCharacter(characterID string) (*ccv3.CharacterCardV3, error) {
	filePath := fmt.Sprintf("characters/%s.json", characterID)

	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Error().Err(err).Str("file_path", filePath).Msg("Failed to read character file")
		return nil, fmt.Errorf("character not found")
	}

	var card ccv3.CharacterCardV3
	if err := json.Unmarshal(data, &card); err != nil {
		log.Error().Err(err).Str("file_path", filePath).Msg("Failed to parse character json")
		return nil, fmt.Errorf("failed to parse character card")
	}

	return &card, nil
}

func buildPrompt(card *ccv3.CharacterCardV3) string {
	r := strings.NewReplacer(
		"{{char}}", card.Data.Name,
		"{{user}}", "User",
	)
	return r.Replace(card.Data.SystemPrompt)
}

func main() {
	setupLogger()
	info, ok := debug.ReadBuildInfo()
	if !ok {
		log.Warn().Msg("Failed to read build info.")
	} else {
		log.Info().Msg("xitu " + info.Main.Version + " initializing...")
	}

	if err := godotenv.Load(); err != nil {
		log.Info().Msg(".env not found, reading from environment.")
	}

	baseURL := os.Getenv("OPENAI_BASE_URL")
	apiKey := os.Getenv("OPENAI_API_KEY")
	model := os.Getenv("OPENAI_MODEL")

	if baseURL == "" || apiKey == "" || model == "" {
		log.Fatal().Msg("OpenAI API configuration is missing. Please set OPENAI_BASE_URL, OPENAI_API_KEY, and OPENAI_MODEL in your .env file.")
	}

	r := gin.New()
	r.Use(GinLogger())
	r.Use(gin.Recovery())

	r.GET("/", func(c *gin.Context) {
		log.Info().Msg("OpenAI API call received")
		c.String(http.StatusOK, "AI Chat Server is running.")
	})

	r.GET("/api/character/:id", func(c *gin.Context) {
		characterID := c.Param("id")

		card, err := loadCharacter(characterID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"name": card.Data.Name,
		})
	})

	r.POST("/api/chat", func(c *gin.Context) {
		var req ChatRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Warn().Err(err).Msg("Invalid request body")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		card, err := loadCharacter(req.CharacterID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		finalSystemPrompt := buildPrompt(card)
		messages := []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: finalSystemPrompt,
			},
		}
		messages = append(messages, req.History...)

		config := openai.DefaultConfig(apiKey)
		config.BaseURL = baseURL

		client := openai.NewClientWithConfig(config)
		resp, err := client.CreateChatCompletion(
			context.Background(),
			openai.ChatCompletionRequest{
				Model:       model,
				Messages:    messages,
				Temperature: 1,
			},
		)
		if err != nil {
			log.Error().Err(err).Str("character_id", req.CharacterID).Msg("OpenAI API call failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get response from AI"})
			return
		}

		if len(resp.Choices) == 0 {
			log.Error().Str("character_id", req.CharacterID).Msg("AI returned an empty response")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "AI returned an empty response"})
			return
		}

		aiReply := resp.Choices[0].Message.Content
		c.JSON(http.StatusOK, gin.H{"reply": aiReply})
	})

	log.Info().Msg("Starting server on port 8080...")
	if err := r.Run(":8080"); err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}
