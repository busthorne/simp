package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"
)

func tag(props ...string) string {
	for i, prop := range props {
		if prop == "" {
			props[i] = "omitempty"
		}
	}
	return "`json:\"" + strings.Join(props, ",") + `"` + "`"
}

// Simple struct field for clean code generation
type StructField struct {
	Name         string
	Type         string // The target type for the generated struct (e.g., "*Content")
	OriginalType string // The original type from the genai package (e.g., "*genai.Content")
	JSONName     string
}

// StructInfo represents a struct from the genai package that we'll mirror
type StructInfo struct {
	Name    string
	Comment string
	Fields  []StructField
}

// Templates for the code generation
var templates = map[string]string{
	"fileHeader":      fileHeader,
	"enumTypes":       enumTypes,
	"contentType":     contentType,
	"partTypes":       partTypes,
	"structDef":       structDef,
	"helperFunctions": helperFunctions,
	"expireTimeType":  expireTimeType,
}

func main() {
	// Parse command-line arguments
	specifiedPath := ""
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		specifiedPath = os.Args[1]
	}

	// Find the genai package
	pkgPath, err := findPackagePath(specifiedPath)
	if err != nil {
		log.Fatalf("Could not find genai package. Please specify the path explicitly.")
	}

	log.Printf("Using genai package at: %s", pkgPath)

	// Parse the genai package
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, pkgPath, nil, parser.ParseComments)
	if err != nil {
		log.Fatalf("Failed to parse package: %v", err)
	}

	// Collect all struct types from the package
	structs := []StructInfo{}

	// Process structs into a cleaner format
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				// Look for type declarations
				decl, ok := n.(*ast.GenDecl)
				if !ok || decl.Tok != token.TYPE {
					return true
				}

				// Find struct type specs
				for _, spec := range decl.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}

					structType, ok := typeSpec.Type.(*ast.StructType)
					if !ok {
						continue
					}

					// Skip some structs - we'll handle them specially or they have issues
					name := typeSpec.Name.Name
					// KEEP: "Content", "FunctionDeclaration", "Tool"
					// SKIP: "ExpireTimeOrTTL", "BlockedError", "SafetySetting", "CachedContentToUpdate", "withREST"
					if name == "ExpireTimeOrTTL" || name == "BlockedError" || name == "SafetySetting" || name == "CachedContentToUpdate" || name == "withREST" {
						log.Printf("Skipping explicitly excluded type: %s", name)
						return true
					}

					// Check for unexported fields within the struct
					hasUnexportedFields := false
					if structType.Fields != nil {
						for _, field := range structType.Fields.List {
							if len(field.Names) > 0 && !unicode.IsUpper([]rune(field.Names[0].Name)[0]) {
								hasUnexportedFields = true
								break
							}
						}
					}

					// Check for fields with unexported types
					hasUnexportedTypeField := false
					if structType.Fields != nil {
						for _, field := range structType.Fields.List {
							fieldTypeStr := formatType(field.Type)
							baseType := getBaseTypeIdentifier(fieldTypeStr)

							// Check if the base type identifier starts with a lowercase letter
							// and is not a known basic Go type.
							if len(baseType) > 0 && unicode.IsLower([]rune(baseType)[0]) && !isKnownBasicType(baseType) {
								hasUnexportedTypeField = true
								log.Printf("--- Struct %s: Field %s has potentially unexported base type %s (%s)", name, field.Names[0].Name, baseType, fieldTypeStr)
								break
							}
						}
					}

					// Combine skip conditions
					if hasUnexportedFields || hasUnexportedTypeField {
						if hasUnexportedFields {
							log.Printf("Skipping struct %s due to unexported fields", name)
						} else {
							log.Printf("Skipping struct %s due to fields with unexported types", name)
						}
						return true
					}

					// Get comment
					comment := ""
					if decl.Doc != nil {
						// Format multi-line comments correctly
						lines := strings.Split(decl.Doc.Text(), "\n")
						var sb strings.Builder
						for _, line := range lines {
							line = strings.TrimSpace(line)
							if line == "" {
								continue
							}
							if sb.Len() > 0 {
								sb.WriteString(" ")
							}
							sb.WriteString(line)
						}
						comment = sb.String()
					}

					// Process fields
					fields := []StructField{}
					if structType.Fields != nil {
						for _, field := range structType.Fields.List {
							if len(field.Names) == 0 {
								continue
							}

							fieldName := field.Names[0].Name
							originalFieldType := formatType(field.Type)
							// Determine target type by removing potential genai prefix
							// This assumes we want unqualified types in the generated struct
							targetFieldType := strings.TrimPrefix(originalFieldType, "genai.")

							// Create camelCase JSON name
							jsonName := fieldName
							if len(jsonName) > 0 {
								runes := []rune(jsonName)
								runes[0] = unicode.ToLower(runes[0])
								jsonName = string(runes)
							}

							fields = append(fields, StructField{
								Name:         fieldName,
								Type:         targetFieldType,   // Use potentially unqualified type
								OriginalType: originalFieldType, // Store the original formatted type
								JSONName:     jsonName,
							})
						}
					}

					// DEBUG: Print processed fields for struct
					// log.Printf("--- Processing struct: %s", name)
					// for _, f := range fields {
					// 	log.Printf("     Field: Name=%s, Type=%s, OriginalType=%s, JSONName=%s", f.Name, f.Type, f.OriginalType, f.JSONName)
					// }

					structs = append(structs, StructInfo{
						Name:    name,
						Comment: comment,
						Fields:  fields,
					})
				}
				return true
			})
		}
	}

	// Output directory
	outDir := "./vertexjson"
	if len(os.Args) > 2 {
		outDir = os.Args[2]
	}

	// Create output directory if it doesn't exist
	err = os.MkdirAll(outDir, 0755)
	if err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Generate mirror structs with JSON tags
	outPath := filepath.Join(outDir, "vertexjson.gen.go")

	// Write to a buffer first
	var buf bytes.Buffer
	apply := func(templateName string) {
		tmpl, ok := templates[templateName]
		if !ok {
			log.Fatalf("Template %q not found", templateName)
		}
		t := template.Must(template.New(templateName).Funcs(template.FuncMap{"tag": tag}).Parse(tmpl))
		if err := t.Execute(&buf, nil); err != nil {
			log.Fatalf("Failed to execute template %s: %v", templateName, err)
		}
	}

	apply("fileHeader")
	apply("enumTypes")
	apply("expireTimeType")
	apply("partTypes")

	structTemplate := template.Must(template.New("structDef").Funcs(template.FuncMap{"tag": tag}).Parse(templates["structDef"]))

	// Generate each struct
	for _, s := range structs {
		if err := structTemplate.Execute(&buf, s); err != nil {
			log.Fatalf("Failed to execute template for struct %s: %v", s.Name, err)
		}
	}

	apply("helperFunctions")

	// Write the code to the file
	if err := os.WriteFile(outPath, buf.Bytes(), 0644); err != nil {
		log.Fatalf("Failed to write file: %v", err)
	}

	// Format the code using gofmt directly
	cmd := exec.Command("gofmt", "-w", outPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("Warning: Failed to format code with gofmt: %v", err)
		log.Printf("gofmt output: %s", output)
	} else {
		log.Printf("Successfully formatted %s with gofmt", outPath)
	}

	fmt.Printf("Generated mirror structs in %s\n", outPath)
}

// formatType formats an AST expression as a Go type
func formatType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", formatType(t.X), t.Sel.Name)
	case *ast.StarExpr:
		return fmt.Sprintf("*%s", formatType(t.X))
	case *ast.ArrayType:
		return fmt.Sprintf("[]%s", formatType(t.Elt))
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", formatType(t.Key), formatType(t.Value))
	case *ast.InterfaceType:
		return "interface{}"
	default:
		return fmt.Sprintf("/* unsupported type %T */", expr)
	}
}

// Helper function to check for known basic Go types
var knownBasicTypes = map[string]bool{
	"string": true, "int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true, "uintptr": true,
	"float32": true, "float64": true, "complex64": true, "complex128": true,
	"bool": true, "byte": true, "rune": true, "error": true, "any": true, "interface{}": true,
}

func isKnownBasicType(typeName string) bool {
	return knownBasicTypes[typeName]
}

// Helper function to extract the base type identifier from a type string
// Example: "[]*time.Time" -> "Time", "map[string]any" -> "any", "byte" -> "byte"
func getBaseTypeIdentifier(typeStr string) string {
	// Remove leading slice/pointer/map syntax
	typeStr = strings.TrimPrefix(typeStr, "[]")
	typeStr = strings.TrimPrefix(typeStr, "*")
	if strings.HasPrefix(typeStr, "map[") {
		if valIdx := strings.Index(typeStr, "]"); valIdx != -1 {
			typeStr = typeStr[valIdx+1:]
			typeStr = strings.TrimPrefix(typeStr, "*") // Handle *mapValue
		}
	}

	// Get the last part after a dot, if any
	if idx := strings.LastIndex(typeStr, "."); idx != -1 {
		typeStr = typeStr[idx+1:]
	}
	return typeStr
}

// findPackagePath locates the genai package using Go modules
func findPackagePath(specifiedPath string) (string, error) {
	// If a path was explicitly specified, use it
	if specifiedPath != "" {
		if _, err := os.Stat(specifiedPath); err == nil {
			return specifiedPath, nil
		}
	}

	// Try to find the module path using 'go list'
	cmd := exec.Command("go", "list", "-m", "-json", "cloud.google.com/go/vertexai")
	output, err := cmd.Output()
	if err == nil {
		var result struct {
			Dir string `json:"Dir"`
		}
		if err := json.Unmarshal(output, &result); err == nil && result.Dir != "" {
			// Module found, check that genai directory exists
			genaiPath := filepath.Join(result.Dir, "genai")
			if _, err := os.Stat(genaiPath); err == nil {
				return genaiPath, nil
			}
		}
	}

	return "", fmt.Errorf("could not find genai package; please specify path explicitly")
}
