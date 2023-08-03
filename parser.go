package main

import (
	"encoding/json"
	"math"
	"os"
	"strings"

	"github.com/go-xmlfmt/xmlfmt"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v3"
)

type Parser = func([]byte) ([]string, error)

type Line struct {
	Content   string
	Indention int
	Index     int
	CanFold   bool // do I have things under me with a larger indention?
	IsFolded  bool // am I folded?
	Hidden    bool // am I hidden? i.e. is a parent folded
}

var parsers = map[string]Parser{
	"json": readAndFormatJSON,
	"yaml": readAndFormatYAML,
	"xml":  readAndFormatXML,
	"raw":  readAndFormatRaw,
}

var parserOrder = []string{
	"json",
	"yaml",
	"xml",
	"raw",
}

func ReadAndFormat(path string, overrideOrder ...string) (lines []*Line, format string, smIndent int, err error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, "", 0, errors.Wrap(err, "unable to read file")
	}

	list := parserOrder
	if len(overrideOrder) > 0 {
		list = overrideOrder
	}
	for _, format = range list {
		parser, ok := parsers[strings.ToLower(format)]
		if !ok {
			return nil, "", 0, errors.New("unknown format")
		}
		formatted, err := parser(content)
		if err != nil {
			continue
		}

		lines, smIndent = process(formatted)

		return lines, format, smIndent, nil
	}

	return nil, "", 0, errors.New("unable to parse file")
}

func process(content []string) (lines []*Line, smallestIndent int) {
	lines = []*Line{}
	smallestIndent = math.MaxInt32

	var prevLine *Line
	for i, line := range content {
		indent := 0
		brk := false
		for i := 0; i < len(line) && !brk; i++ {
			switch line[i] {
			case ' ':
				indent++
			case '\t':
				indent += Cfg.TabSize
			default:
				brk = true
			}
		}

		if prevLine != nil {
			prevLine.CanFold = prevLine.Indention < indent
		}

		if indent < smallestIndent {
			smallestIndent = indent
		}

		prevLine = &Line{
			Content:   line,
			Indention: indent,
			Index:     i,
		}

		lines = append(lines, prevLine)
	}

	return lines, smallestIndent
}

func readAndFormatJSON(content []byte) (lines []string, err error) {
	var raw json.RawMessage

	err = json.Unmarshal(content, &raw)
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse")
	}

	formatted, err := json.MarshalIndent(raw, "", strings.Repeat(" ", Cfg.TabSize))
	if err != nil {
		return nil, errors.Wrap(err, "unable to format")
	}

	return splitLines(formatted), nil
}

func readAndFormatYAML(content []byte) (lines []string, err error) {
	raw := map[interface{}]interface{}{}

	err = yaml.Unmarshal(content, &raw)
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse")
	}

	formatted, err := yaml.Marshal(raw)
	if err != nil {
		return nil, errors.Wrap(err, "unable to format")
	}

	return splitLines(formatted), nil
}

func readAndFormatXML(content []byte) (lines []string, err error) {
	formatted := xmlfmt.FormatXML(string(content), "", "  ")

	return splitLines([]byte(formatted)), nil
}

func readAndFormatRaw(content []byte) (lines []string, err error) {
	return splitLines(content), nil
}

func splitLines(content []byte) (lines []string) {
	lines = []string{}
	from := 0
	prev := byte(0)

	for i, b := range content {
		if b == '\n' {
			if prev == '\r' {
				lines = append(lines, string(content[from:i-1]))
			} else {
				lines = append(lines, string(content[from:i]))
			}
			from = i + 1
		}
		prev = b
	}

	if len(content[from:]) > 0 {
		lines = append(lines, string(content[from:]))
	}

	return lines
}
