// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package paperedge generates reproducible, schema-valid JSON cases for
// exercising .paper templates across structural and layout boundaries.
package paperedge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"

	"github.com/cssbruno/paperrune/internal/papercompile"
)

const (
	defaultCount        = 12
	defaultMaxListItems = 64
	maxCount            = 256
	maxListItems        = 1000
	maxCaseBytes        = 8 << 20
)

// Options bounds and seeds generation. The same schema and options always
// produce byte-identical cases.
type Options struct {
	Count        uint32
	Seed         int64
	MaxListItems uint32
}

// Case is one replayable JSON input. Digest is the lowercase SHA-256 of JSON.
type Case struct {
	Name   string `json:"name"`
	Digest string `json:"sha256"`
	JSON   []byte `json:"-"`
}

type profile uint8

const (
	profileEmptyText profile = iota
	profileMinimal
	profileWhitespaceText
	profileMultilineText
	profileLongText
	profileLongUnbroken
	profileDenseLists
	profileUnicode
	profilePunctuation
	profileNumericBounds
	profileRandom
)

// Generate produces fixed structural boundaries first, then seeded random
// cases. It never exceeds the schema's list bounds or the explicit safety cap.
func Generate(schema papercompile.SchemaDescriptor, options Options) ([]Case, error) {
	if schema.Name == "" || schema.Kind != papercompile.SchemaObject {
		return nil, errors.New("paperedge: selected schema must be a named object")
	}
	if options.Count == 0 {
		options.Count = defaultCount
	}
	if options.Count > maxCount {
		return nil, fmt.Errorf("paperedge: count exceeds %d", maxCount)
	}
	if options.MaxListItems == 0 {
		options.MaxListItems = defaultMaxListItems
	}
	if options.MaxListItems > maxListItems {
		return nil, fmt.Errorf("paperedge: max list items exceeds %d", maxListItems)
	}
	random := rand.New(rand.NewSource(options.Seed)) // #nosec G404 -- deterministic test-data generation, not security.
	cases := make([]Case, 0, options.Count)
	for index := uint32(0); index < options.Count; index++ {
		mode, name := caseProfile(index)
		value := generateObject(schema.Fields, mode, random, options.MaxListItems, 0)
		encoded, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("paperedge: encode %s: %w", name, err)
		}
		encoded = append(encoded, '\n')
		if len(encoded) > maxCaseBytes {
			return nil, fmt.Errorf("paperedge: case %s exceeds %d bytes", name, maxCaseBytes)
		}
		digest := sha256.Sum256(encoded)
		cases = append(cases, Case{Name: name, Digest: hex.EncodeToString(digest[:]), JSON: encoded})
	}
	return cases, nil
}

func caseProfile(index uint32) (profile, string) {
	switch index {
	case 0:
		return profileEmptyText, "empty-text"
	case 1:
		return profileMinimal, "minimal"
	case 2:
		return profileWhitespaceText, "whitespace-text"
	case 3:
		return profileMultilineText, "multiline-text"
	case 4:
		return profileLongText, "long-text"
	case 5:
		return profileLongUnbroken, "long-unbroken-string"
	case 6:
		return profileDenseLists, "dense-lists"
	case 7:
		return profileUnicode, "unicode-pt-br"
	case 8:
		return profilePunctuation, "punctuation-and-escaping"
	case 9:
		return profileNumericBounds, "numeric-bounds"
	default:
		return profileRandom, fmt.Sprintf("random-%03d", index-9)
	}
}

func generateObject(fields []papercompile.FieldDescriptor, mode profile, random *rand.Rand, listCap, depth uint32) map[string]any {
	result := make(map[string]any, len(fields))
	for _, field := range fields {
		if !field.Required && !includeOptional(mode, random) {
			continue
		}
		result[field.Name] = generateField(field, mode, random, listCap, depth+1)
	}
	return result
}

func includeOptional(mode profile, random *rand.Rand) bool {
	switch mode {
	case profileMinimal:
		return false
	case profileEmptyText, profileWhitespaceText, profileMultilineText, profileLongText,
		profileLongUnbroken, profileDenseLists, profileUnicode, profilePunctuation, profileNumericBounds:
		return true
	default:
		return random.Intn(2) == 0
	}
}

func generateField(field papercompile.FieldDescriptor, mode profile, random *rand.Rand, listCap, depth uint32) any {
	switch field.Kind {
	case papercompile.SchemaString:
		return generatedString(mode, random, field.Name, depth)
	case papercompile.SchemaNumber:
		return generatedNumber(mode, random)
	case papercompile.SchemaBool:
		return mode != profileMinimal && random.Intn(2) == 0
	case papercompile.SchemaObject:
		return generateObject(field.Fields, mode, random, listCap, depth)
	case papercompile.SchemaList:
		count := generatedListCount(field.MaxItems, mode, random, listCap)
		items := make([]any, 0, count)
		for index := uint32(0); index < count; index++ {
			item := papercompile.FieldDescriptor{Kind: field.ItemKind, Required: field.ItemRequired, Fields: field.Fields, Name: field.Name}
			items = append(items, generateField(item, mode, random, listCap, depth+1))
		}
		return items
	default:
		return nil
	}
}

func generatedListCount(schemaMax uint32, mode profile, random *rand.Rand, listCap uint32) uint32 {
	limit := schemaMax
	if limit > listCap {
		limit = listCap
	}
	if limit == 0 || mode == profileMinimal {
		return 0
	}
	switch mode {
	case profileEmptyText, profileWhitespaceText, profileMultilineText, profileLongText,
		profileLongUnbroken, profileUnicode, profilePunctuation, profileNumericBounds:
		if limit > 2 {
			return 2
		}
		return limit
	case profileDenseLists:
		return limit
	default:
		return uint32(random.Intn(int(limit) + 1)) // #nosec G115 -- limit is capped at 1000.
	}
}

func generatedString(mode profile, random *rand.Rand, field string, depth uint32) string {
	switch mode {
	case profileEmptyText:
		return ""
	case profileMinimal:
		return "x"
	case profileWhitespaceText:
		return "     \n  "
	case profileMultilineText:
		return "Linha 1: " + field + "\nLinha 2: valor complementar\n\nLinha 4: após linha vazia"
	case profileLongText:
		repetitions := 4
		if depth >= 3 {
			repetitions = 18
		}
		return strings.Repeat("Long "+field+" value ", repetitions)
	case profileLongUnbroken:
		length := 24
		if depth >= 3 {
			length = 256
		}
		return strings.Repeat("W", length)
	case profileDenseLists:
		return fmt.Sprintf("%s-%08d", field, random.Int31())
	case profileUnicode:
		return "João da Conceição — São José, ação clínica nº 123"
	case profilePunctuation:
		return `"quoted" \\ slash / percent % parentheses () brackets [] braces {} <>& #;:,.!?`
	case profileNumericBounds:
		return field + " boundary value"
	default:
		words := []string{"alpha", "beta", "gamma", "delta", "clinic", "laboratory", "result", "patient", "sample", "reference"}
		count := 1 + random.Intn(12)
		parts := make([]string, count)
		for index := range parts {
			parts[index] = words[random.Intn(len(words))]
		}
		if depth%3 == 0 && random.Intn(4) == 0 {
			parts = append(parts, "São", "José")
		}
		return strings.Join(parts, " ")
	}
}

func generatedNumber(mode profile, random *rand.Rand) json.Number {
	switch mode {
	case profileEmptyText, profileMinimal, profileWhitespaceText:
		return json.Number("0")
	case profileLongText:
		return json.Number("-999999.999")
	case profileMultilineText, profileLongUnbroken, profilePunctuation, profileUnicode:
		return json.Number("12.5")
	case profileDenseLists:
		return json.Number("999999999.9999")
	case profileNumericBounds:
		return json.Number("-999999999999.9999")
	default:
		whole := random.Int63n(2_000_001) - 1_000_000
		fraction := random.Intn(10_000)
		if fraction == 0 {
			return json.Number(fmt.Sprintf("%d", whole))
		}
		return json.Number(strings.TrimRight(fmt.Sprintf("%d.%04d", whole, fraction), "0"))
	}
}
