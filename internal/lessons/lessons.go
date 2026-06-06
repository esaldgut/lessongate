// Package lessons parses the curated lesson registry
// (feedback_lessons_learned.md) into structured Lesson values. The registry is
// the LOW-NDA-density source the pipeline consumes — already human-distilled to
// patterns — instead of the raw PR diff (threat C1).
//
// Format per entry: "## Lesson N: <title>" followed by a description, a
// "**Why:**" section, a "**Do instead:**"/"**How to apply:**" section, and a
// code example, separated by "---".
//
// Built in Fase 2. This file declares the contract.
package lessons

import (
	"regexp"
	"strconv"
	"strings"
)

// Lesson is one parsed entry from the registry.
type Lesson struct {
	Number int
	Title  string
	Body   string // full markdown body (description + Why + How + code)
}

// headingRe matches a lesson heading line: "## Lesson N: <title>".
var headingRe = regexp.MustCompile(`^## Lesson (\d+):\s*(.*)$`)

// Parse extracts all lessons from registry markdown content. It splits on
// "## Lesson N:" heading lines that occur OUTSIDE fenced code blocks, so an
// example heading inside a ``` block never creates a phantom lesson. Content
// before the first real heading (a file preamble) is ignored.
func Parse(markdown string) ([]Lesson, error) {
	var out []Lesson
	var cur *Lesson
	var body strings.Builder
	inFence := false

	flush := func() {
		if cur != nil {
			cur.Body = strings.TrimSpace(body.String())
			out = append(out, *cur)
		}
		body.Reset()
	}

	for line := range strings.SplitSeq(markdown, "\n") {
		trimmed := strings.TrimSpace(line)

		// Track fenced code state on lines starting with ``` (any info string).
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			if cur != nil {
				body.WriteString(line)
				body.WriteByte('\n')
			}
			continue
		}

		if !inFence {
			if m := headingRe.FindStringSubmatch(line); m != nil {
				flush() // close the previous lesson, if any
				n, _ := strconv.Atoi(m[1])
				cur = &Lesson{Number: n, Title: strings.TrimSpace(m[2])}
				continue // the heading line itself is not part of the body
			}
		}

		if cur != nil {
			body.WriteString(line)
			body.WriteByte('\n')
		}
	}
	flush()

	return out, nil
}
