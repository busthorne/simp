# Vertex AI JSON Struct Generator

This tool generates Go structs that mirror the Vertex AI `genai` types but include JSON tags, along with conversion functions. This solves the problem of JSON (un)marshaling with the Vertex AI Go SDK.

## Problem

The Google Vertex AI Go SDK (`cloud.google.com/go/vertexai/genai`) uses struct types that don't have JSON tags, making it difficult to directly (un)marshal JSON from API responses. This generator creates mirror structs with proper JSON tags and bidirectional conversion functions.

## Usage

### Generate the structs

```bash
# Default paths
go run cmd/genvertex/main.go

# Or specify custom paths
go run cmd/genvertex/main.go /path/to/vertexai/genai /path/to/output/dir
```

This will generate a `vertex_gen.go` file in the output directory with all the mirror structs and conversion functions.

### Using the generated structs

```go
import (
    "encoding/json"
    "your-project/internal/vertexjson" // Update with your import path
    "cloud.google.com/go/vertexai/genai"
)

// Unmarshal JSON directly into our generated structs
var response vertexjson.GenerateContentResponse
json.Unmarshal(jsonBytes, &response)

// Convert to SDK types for use with the SDK
genaiResponse := response.To()

// Use with the SDK
for _, candidate := range genaiResponse.Candidates {
    // Use SDK types normally
}

// Or convert from SDK types to JSON-friendly structs
sdkResponse, _ := model.GenerateContent(ctx, genai.Text("Hello"))
jsonResp := vertexjson.FromGenerateContentResponse(sdkResponse)

// Marshal to JSON
jsonBytes, _ := json.Marshal(jsonResp)
```

## Benefits

1. **Clean unmarshaling** - Parse JSON API responses directly
2. **Type safety** - Full type safety with Go's type system
3. **Bidirectional conversion** - Convert between JSON structs and SDK types
4. **Maintainability** - Regenerate when the SDK updates

## Implementation Details

This tool:

1. Parses the `genai` package using Go's AST tools
2. Extracts all struct definitions
3. Generates mirror structs with JSON tags
4. Creates conversion functions between the two

The generated code maintains all type relationships and field names from the original SDK. 
