package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/busthorne/simp/driver"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/sashabaranov/go-openai"
)

func gateway() {
	f := fiber.New()
	f.Use(cors.New())
	v1 := f.Group("/v1")
	v1.Get("/ping", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	v1.Get("/models", func(c *fiber.Ctx) error {
		return c.JSON(driver.Drivers)
	})
	v1.Post("/embeddings", func(c *fiber.Ctx) error {
		var req openai.EmbeddingRequest
		if err := c.BodyParser(&req); err != nil {
			return badRequest(c, err)
		}
		drv, model, err := findWaldo(string(req.Model))
		if err != nil {
			return badRequest(c, err)
		}
		req.Model = openai.EmbeddingModel(model.Name)
		resp, err := drv.Embed(c.Context(), req)
		if err != nil {
			return internalError(c, err)
		}
		return c.JSON(resp)
	})
	v1.Post("/chat/completions", func(c *fiber.Ctx) error {
		var req openai.ChatCompletionRequest
		if err := c.BodyParser(&req); err != nil {
			return badRequest(c, err)
		}
		drv, model, err := findWaldo(req.Model)
		if err != nil {
			return badRequest(c, err)
		}
		req.Model = model.Name
		resp, err := drv.Complete(c.Context(), req)
		if err != nil {
			return internalError(c, err)
		}
		if !req.Stream {
			return c.JSON(resp)
		}
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("Transfer-Encoding", "chunked")
		c.Status(200)
		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			for ch := range resp.Stream {
				c := ch.Choices[0]

				var delta openai.ChatCompletionStreamChoiceDelta
				switch c.FinishReason {
				case "":
					delta = openai.ChatCompletionStreamChoiceDelta{
						Role:    "assistant",
						Content: c.Delta.Content,
					}
				case "error":
					goto done
				default:
				}
				fmt.Fprint(w, "data: ")
				json.NewEncoder(w).Encode(openai.ChatCompletionStreamResponse{
					Object: "chat.completion.chunk",
					Choices: []openai.ChatCompletionStreamChoice{{
						FinishReason: c.FinishReason,
						Delta:        delta}},
					Created: time.Now().Unix(),
				})
				fmt.Fprintln(w)
				w.Flush()
			}
		done:
			fmt.Fprintln(w)
			fmt.Fprintf(w, "data: [DONE]\n")
			fmt.Fprintln(w)
			w.Flush()
		})
		return nil
	})
	v1.Get("/batches", func(c *fiber.Ctx) error {
		return badRequest(c, "not implemented")
	})
	v1.Get("/batches/:id", func(c *fiber.Ctx) error {
		return badRequest(c, "not implemented")
	})
	v1.Post("/batches", func(c *fiber.Ctx) error {
		return badRequest(c, "not implemented")
	})
	v1.Post("/batches/:id/cancel", func(c *fiber.Ctx) error {
		return badRequest(c, "not implemented")
	})
	addr := strings.Split(cfg.Daemon.ListenAddr, "://")
	switch addr[0] {
	case "http":
		f.Listen(addr[1])
	case "https":
		fmt.Println("HTTPS is not supported yet.")
		exit(1)
	default:
		fmt.Printf("unknown protocol: %s\n", addr[0])
		exit(1)
	}
}

func badRequest(c *fiber.Ctx, err any) error {
	return c.Status(fiber.StatusBadRequest).
		JSON(fiber.Map{"error": err})
}

func internalError(c *fiber.Ctx, err any) error {
	return c.Status(fiber.StatusInternalServerError).
		JSON(fiber.Map{"error": err})
}
