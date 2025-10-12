package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	"github.com/cloudwindy/xitu/st"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

type ChatRequest struct {
	CharacterID string                         `json:"character_id" binding:"required"`
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

var cache = make(map[string]st.Card)

func loadCharacter(characterID string) (st.Card, error) {
	if card, ok := cache[characterID]; ok {
		return card, nil
	}
	filePath := fmt.Sprintf("characters/%s.json", characterID)

	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Error().Err(err).Str("file_path", filePath).Msg("Failed to read character file")
		return nil, fmt.Errorf("character not found")
	}

	card, err := st.NewCard(data)
	if err != nil {
		log.Error().Err(err).Str("file_path", filePath).Msg("Failed to parse character json")
		return nil, fmt.Errorf("failed to parse character card")
	}
	cache[characterID] = card

	return card, nil
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Warn().Err(err).Msg("Failed to read env file, reading from environment")
	}
	mode := os.Getenv("XITU_MODE")
	if mode == "debug" {
		gin.SetMode(gin.DebugMode)
	} else if mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		log.Warn().Msg("XITU_MODE not valid, using 'debug'")
		gin.SetMode(gin.DebugMode)
	}
	setupLogger()

	info, ok := debug.ReadBuildInfo()
	if !ok {
		log.Warn().Msg("Failed to read build info")
	} else {
		log.Info().Msg("xitu " + info.Main.Version)
	}

	baseURL := os.Getenv("OPENAI_BASE_URL")
	apiKey := os.Getenv("OPENAI_API_KEY")
	model := os.Getenv("OPENAI_MODEL")

	if baseURL == "" || apiKey == "" || model == "" {
		log.Fatal().Msg("OpenAI API configuration is missing. Please set OPENAI_BASE_URL, OPENAI_API_KEY, and OPENAI_MODEL in your .env file")
	}

	r := gin.New()
	r.Use(GinLogger())
	r.Use(gin.Recovery())

	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "XITU is running")
	})

	r.GET("/api/character/:id", func(c *gin.Context) {
		characterID := c.Param("id")

		card, err := loadCharacter(characterID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"name":         card.GetData().Name,
			"version":      card.GetData().CharacterVersion,
			"creator":      card.GetData().Creator,
			"creatornotes": card.GetData().CreatorNotes,
			"tags":         card.GetData().Tags,
			"firstmes":     card.GetData().FirstMes,
		})
	})

	if gin.Mode() == gin.DebugMode {
		r.POST("/api/debug/chat", func(c *gin.Context) {
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

			messages, err := card.Apply(req.History)
			if err != nil {
				log.Error().Err(err).Str("character_id", req.CharacterID).Msg("Failed to apply messages")
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"messages": messages})
		})
	}

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

		config := openai.DefaultConfig(apiKey)
		config.BaseURL = baseURL

		messages, err := card.Apply(req.History)
		if err != nil {
			log.Error().Err(err).Str("character_id", req.CharacterID).Msg("Failed to apply messages")
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

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

	if err := r.Run(":8080"); err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}
