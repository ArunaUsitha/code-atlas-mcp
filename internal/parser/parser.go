package parser

import (
	"context"
	"fmt"
	"os"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/python"
)

type Symbol struct {
	Name          string
	QualifiedName string
	Kind          string // "class", "function"
	StartLine     int
	EndLine       int
	Signature     string
}

func ParseFile(filePath string, langType string) ([]Symbol, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var lang *sitter.Language
	var queryStr string

	switch strings.ToLower(langType) {
	case "python":
		lang = python.GetLanguage()
		queryStr = `
			(function_definition
				name: (identifier) @func.name
				parameters: (parameters) @func.params) @func.def
			(class_definition
				name: (identifier) @class.name) @class.def
		`
	case "go":
		lang = golang.GetLanguage()
		queryStr = `
			(function_declaration
				name: (identifier) @func.name
				parameter_list: (parameter_list) @func.params) @func.def
			(method_declaration
				name: (field_identifier) @func.name
				parameter_list: (parameter_list) @func.params) @func.def
			(type_spec
				name: (type_identifier) @class.name
				type: (struct_type)) @class.def
		`
	default:
		return nil, fmt.Errorf("unsupported language: %s", langType)
	}

	parser := sitter.NewParser()
	parser.SetLanguage(lang)

	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse tree: %w", err)
	}

	rootNode := tree.RootNode()
	query, err := sitter.NewQuery([]byte(queryStr), lang)
	if err != nil {
		return nil, fmt.Errorf("failed to create query: %w", err)
	}

	cursor := sitter.NewQueryCursor()
	cursor.Exec(query, rootNode)

	var symbols []Symbol
	for {
		m, ok := cursor.NextMatch()
		if !ok {
			break
		}

		for _, capture := range m.Captures {
			captureName := query.CaptureNameForId(capture.Index)
			node := capture.Node

			if captureName == "func.def" {
				nameNode := node.ChildByFieldName("name")
				name := ""
				if nameNode != nil {
					name = nameNode.Content(content)
				} else {
					// Fallback for methods or declaration variations
					continue
				}

				paramsNode := node.ChildByFieldName("parameter_list")
				if paramsNode == nil {
					paramsNode = node.ChildByFieldName("parameters")
				}
				signature := ""
				if paramsNode != nil {
					signature = paramsNode.Content(content)
				}

				symbols = append(symbols, Symbol{
					Name:      name,
					Kind:      "function",
					StartLine: int(node.StartPoint().Row) + 1,
					EndLine:   int(node.EndPoint().Row) + 1,
					Signature: signature,
				})
			} else if captureName == "class.def" {
				nameNode := node.ChildByFieldName("name")
				name := ""
				if nameNode != nil {
					name = nameNode.Content(content)
				} else {
					continue
				}

				symbols = append(symbols, Symbol{
					Name:      name,
					Kind:      "class",
					StartLine: int(node.StartPoint().Row) + 1,
					EndLine:   int(node.EndPoint().Row) + 1,
				})
			}
		}
	}

	return symbols, nil
}
