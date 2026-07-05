// audio は、Gemini API (Lyria) を使って音声を生成し、WAVファイルとして保存する CLI ツールです。
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		return err
	}

	// 設定を最小限に。AUDIOのみを要求
	config := &genai.GenerateContentConfig{
		ResponseModalities: []string{"AUDIO"},
		ResponseMIMEType:   "audio/wav",
	}

	slog.Info("generating audio...")
	res, err := client.Models.GenerateContent(ctx, "lyria-3-pro-preview", genai.Text("Hyper Techno track"), config)
	if err != nil {
		return err
	}

	for _, part := range res.Candidates[0].Content.Parts {
		if part.InlineData != nil {
			filename := "output.wav"
			if err := os.WriteFile(filename, part.InlineData.Data, 0644); err != nil {
				return err
			}
			slog.Info("saved!", "file", filename)
			return nil
		}
	}

	return fmt.Errorf("no audio data")
}
