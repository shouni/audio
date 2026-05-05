package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"google.golang.org/genai"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY is not set")
	}

	cc := &genai.ClientConfig{
		APIKey: apiKey,
	}
	client, err := genai.NewClient(ctx, cc)
	if err != nil {
		log.Fatal(err)
	}
	config := &genai.GenerateContentConfig{
		ResponseModalities: []string{"AUDIO", "TEXT"},
		ResponseMIMEType:   "audio/wav",
	}

	result, err := client.Models.GenerateContent(
		ctx,
		"lyria-3-pro-preview",
		genai.Text("An atmospheric ambient track."),
		config,
	)

	if err != nil {
		log.Fatal(err)
	}

	if len(result.Candidates) == 0 || result.Candidates[0].Content == nil || len(result.Candidates[0].Content.Parts) == 0 {
		log.Fatal("no content returned")
	}

	audioCount := 0
	for _, part := range result.Candidates[0].Content.Parts {
		if part.Text != "" {
			fmt.Println(part.Text)
		} else if part.InlineData != nil {
			if part.InlineData.MIMEType != "" && !strings.HasPrefix(part.InlineData.MIMEType, "audio/") {
				log.Printf("Skipping non-audio inline data: %s", part.InlineData.MIMEType)
				continue
			}

			audioCount++
			filename := fmt.Sprintf("test-%d.wav", audioCount)
			err := os.WriteFile(filename, part.InlineData.Data, 0644)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("Audio saved to", filename)
		}
	}

	if audioCount == 0 {
		log.Fatal("no audio data returned")
	}
}
