package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"google.golang.org/genai"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("application terminated with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("GEMINI_API_KEY is not set")
	}

	cc := &genai.ClientConfig{
		APIKey: apiKey,
	}
	client, err := genai.NewClient(ctx, cc)
	if err != nil {
		return fmt.Errorf("failed to create genai client: %w", err)
	}

	config := &genai.GenerateContentConfig{
		ResponseModalities: []string{"AUDIO", "TEXT"},
		ResponseMIMEType:   "audio/wav",
	}

	slog.Info("generating content", "model", "lyria-3-pro-preview")
	result, err := client.Models.GenerateContent(
		ctx,
		"lyria-3-pro-preview",
		genai.Text("An atmospheric ambient track."),
		config,
	)
	if err != nil {
		return fmt.Errorf("failed to generate content: %w", err)
	}

	if len(result.Candidates) == 0 || result.Candidates[0].Content == nil || len(result.Candidates[0].Content.Parts) == 0 {
		return fmt.Errorf("no content returned from API")
	}

	audioCount := 0
	for _, part := range result.Candidates[0].Content.Parts {
		if part.Text != "" {
			fmt.Println(part.Text)
		} else if part.InlineData != nil {
			mimeType := part.InlineData.MIMEType
			if mimeType != "" && !strings.HasPrefix(mimeType, "audio/") {
				slog.Warn("skipping non-audio inline data", "mimeType", mimeType)
				continue
			}

			audioCount++
			filename := fmt.Sprintf("test-%d.wav", audioCount)
			if err := os.WriteFile(filename, part.InlineData.Data, 0644); err != nil {
				return fmt.Errorf("failed to save audio file %s: %w", filename, err)
			}
			slog.Info("audio saved", "filename", filename)
		}
	}

	if audioCount == 0 {
		return fmt.Errorf("no audio data found in the response")
	}

	return nil
}
