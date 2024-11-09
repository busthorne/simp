# Dify Go SDK
This is the Go SDK for the Dify API, which allows you to easily integrate Dify into your Go applications.

## Install
```bash
go get github.com/langgenius/dify-sdk-go
```

## Usage
After installing the SDK, you can use it in your project like this:

```go
package main

import (
	"context"
	"log"
	"strings"

	"github.com/langgenius/dify-sdk-go"
)

func main() {
	var (
		ctx = context.Background()
		c = dify.NewClient("your-dify-server-host", "your-api-key-here")

		req = &dify.ChatMessageRequest{
			Query: "your-question",
			User: "your-user",
		}

		ch chan dify.ChatMessageStreamChannelResponse
		err error
	)

	if ch, err = c.Api().ChatMessagesStream(ctx, req); err != nil {
		return
	}

	var strBuilder strings.Builder

	for {
		select {
		case <-ctx.Done():
			return
		case streamData, isOpen := <-ch:
			if err = streamData.Err; err != nil {
				log.Println(err.Error())
				return
			}
			if !isOpen {
				log.Println(strBuilder.String())
				return
			}

			strBuilder.WriteString(streamData.Answer)
		}
	}
}

```

## License
This SDK is released under the MIT License.