package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/busthorne/simp"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/sashabaranov/go-openai"
)

func listen() *fiber.App {
	nop := func(c *fiber.Ctx) error {
		return notImplemented(c)
	}

	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})
	f.Use(cors.New())
	f.Use(func(c *fiber.Ctx) (err error) {
		if err = c.Next(); err == nil {
			return
		}
		log.Errorf("%s %s %v\n", c.Method(), c.Path(), err)
		if errors.Is(err, simp.ErrBookkeeping) {
			c.Status(fiber.StatusInternalServerError)
			return c.JSON(fiber.Map{"error": fiber.Map{
				"message": "bookkeeping error",
				"type":    "error",
			}})
		} else if c.Response().StatusCode() == fiber.StatusOK {
			c.Status(fiber.StatusBadRequest)
		}
		var errType = "invalid_request_error"
		if un := errors.Unwrap(err); un != nil {
			err = un
		}
		switch err := err.(type) {
		case *openai.APIError:
			errType = err.Type
		}
		return c.JSON(fiber.Map{"error": fiber.Map{
			"message": err.Error(),
			"type":    errType,
		}})
	})
	v1 := f.Group("/v1")
	v1.Get("/ping", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	v1.Get("/models", func(c *fiber.Ctx) error {
		var models []openai.Model
		for _, p := range cfg.Providers {
			for _, m := range p.Models {
				models = append(models, openai.Model{
					ID:     m.Name,
					Object: "model",
				})
				for _, a := range m.Alias {
					models = append(models, openai.Model{
						ID:     a,
						Object: "model",
						Root:   m.Name,
					})
				}
			}
		}
		return c.JSON(openai.ModelsList{Models: models})
	})
	v1.Post("/embeddings", func(c *fiber.Ctx) error {
		var req openai.EmbeddingRequest
		if err := c.BodyParser(&req); err != nil {
			return err
		}
		drv, model, err := findWaldo(string(req.Model))
		if err != nil {
			return err
		}
		log.Debugf("embedding model %s (%T)\n", model.Name, drv)
		req.Model = model.Name
		ctx := context.WithValue(c.Context(), simp.KeyModel, model)
		resp, err := drv.Embed(ctx, req)
		if err != nil {
			return internalError(c, err)
		}
		resp.Model = openai.EmbeddingModel(model.Name)
		resp.Object = "list"
		for i := range resp.Data {
			resp.Data[i].Object = "embedding"
		}
		return c.JSON(resp)
	})
	v1.Post("/chat/completions", func(c *fiber.Ctx) error {
		var req openai.ChatCompletionRequest
		if err := c.BodyParser(&req); err != nil {
			return err
		}
		drv, model, err := findWaldo(req.Model)
		if err != nil {
			return err
		}
		log.Debugf("completion model %s (%T)\n", model.Name, drv)
		req.Model = model.Name
		ctx := context.WithValue(c.Context(), simp.KeyModel, model)
		resp, err := drv.Chat(ctx, req)
		if err != nil {
			return internalError(c, err)
		}
		if !req.Stream {
			resp.Object = "chat.completion"
			resp.Model = model.Name
			return c.JSON(resp)
		}
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("Transfer-Encoding", "chunked")
		c.Status(200)
		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			var ret error
			for chunk := range resp.Stream {
				if len(chunk.Choices) == 0 {
					if chunk.Usage != nil {
						fmt.Fprint(w, "data: ")
						json.NewEncoder(w).Encode(openai.ChatCompletionStreamResponse{
							Object:  "chat.completion.chunk",
							Usage:   chunk.Usage,
							Created: time.Now().Unix(),
						})
						w.Flush()
					}
					continue
				}
				c := chunk.Choices[0]

				if c.FinishReason == "error" {
					ret = chunk.Error
					break
				}
				delta := openai.ChatCompletionStreamChoiceDelta{
					Role:    "assistant",
					Content: c.Delta.Content,
				}
				fmt.Fprint(w, "data: ")
				json.NewEncoder(w).Encode(openai.ChatCompletionStreamResponse{
					Object: "chat.completion.chunk",
					Choices: []openai.ChatCompletionStreamChoice{{
						FinishReason: c.FinishReason,
						Delta:        delta}},
					Created: time.Now().Unix(),
				})
				w.Flush()
			}
			switch err := ret.(type) {
			case nil:
			case *openai.APIError:
				fmt.Fprintf(w, "\ndata: ")
				json.NewEncoder(w).Encode(openai.ErrorResponse{Error: err})
			default:
				fmt.Fprintf(w, "\ndata: ")
				json.NewEncoder(w).Encode(openai.ErrorResponse{
					Error: &openai.APIError{
						Type:    "provider_error",
						Message: err.Error(),
					},
				})
			}
			fmt.Fprintf(w, "data: [DONE]\n")
			w.Flush()
		})
		return nil
	})
	v1.Post("/files", BatchUpload)
	v1.Get("/files/:id/content", BatchReceive)
	v1.Get("/batches", nop)
	v1.Post("/batches", BatchSend)
	v1.Post("/batches/:id/cancel", BatchCancel)
	v1.Get("/batches/:id", BatchRefresh)

	addr := strings.Split(cfg.Daemon.ListenAddr, "://")
	switch addr[0] {
	case "https":
		log.Fatal("HTTPS is not supported yet.")
	case "http":
		log.Infof("listening on %s\n", cfg.Daemon.ListenAddr)
		go func() {
			if err := f.Listen(addr[1]); err != nil {
				log.Fatal(err)
			}
		}()
	default:
		log.Fatalf("unknown protocol: %s\n", addr[0])
	}
	return f
}

func internalError(c *fiber.Ctx, err error) error {
	c.Status(fiber.StatusInternalServerError)
	return err
}

func notImplemented(c *fiber.Ctx) error {
	c.Status(fiber.StatusNotImplemented)
	return simp.ErrNotImplemented
}
