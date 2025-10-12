package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/cloudwindy/xitu/st"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

type ChatRequest struct {
	Model       string                         `json:"model" binding:"required"`
	Messages    []openai.ChatCompletionMessage `json:"messages" binding:"required"`
	Temperature *float32                       `json:"temperature,omitempty"`
	Stream      *bool                          `json:"stream,omitempty"`
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
var validApiKeys = []string{
	"sk-96oyf8lafovtov62", // Example key for testing
}

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
		r.POST("/api/debug/chat/prompt", func(c *gin.Context) {
			req := ChatRequest{}
			if err := c.ShouldBindJSON(&req); err != nil {
				log.Warn().Err(err).Msg("Invalid request body")
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}

			card, err := loadCharacter(req.Model)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}

			messages, err := card.Apply(req.Messages)
			if err != nil {
				log.Error().Err(err).Str("character_id", req.Model).Msg("Failed to apply messages")
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"messages": messages})
		})
	}

	r.POST("/api/v1/chat/completions", func(c *gin.Context) {
		req := ChatRequest{}
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Warn().Err(err).Msg("Invalid request body")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		auth, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok {
			log.Warn().Str("header", c.GetHeader("Authorization")).Msg("Missing or invalid Authorization header")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing or invalid Authorization header"})
			return
		}
		if !slices.Contains(validApiKeys, auth) {
			log.Warn().Str("api_key", auth).Msg("Invalid API key")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
			return
		}

		card, err := loadCharacter(req.Model)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		config := openai.DefaultConfig(apiKey)
		config.BaseURL = baseURL

		messages, err := card.Apply(req.Messages)
		if err != nil {
			log.Error().Err(err).Str("character_id", req.Model).Msg("Failed to apply messages")
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		client := openai.NewClientWithConfig(config)
		openAIReq := openai.ChatCompletionRequest{
			Model:       model,
			Messages:    messages,
			Temperature: 1,
		}
		if req.Temperature != nil {
			openAIReq.Temperature = *req.Temperature
		}
		stream, err := client.CreateChatCompletionStream(context.Background(), openAIReq)
		if err != nil {
			log.Error().Err(err).Str("character_id", req.Model).Msg("OpenAI API call failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get response from AI"})
			return
		}
		c.Stream(func(w io.Writer) bool {
			response, err := stream.Recv()
			if err != nil {
				return false
			}
			c.SSEvent("message", response)
			return true
		})
		if err = stream.Close(); err != nil {
			log.Error().Err(err).Msg("Failed to close stream")
			return
		}
	})

	if err := r.Run(":8080"); err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}
