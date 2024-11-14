// internal/diff/diff.go
package diff

import (
	"bytes"
	"fmt"
)

// Line represents a single line in a diff with its type and content
type Line struct {
	Type    LineType
	Content string
	OldNum  int
	NewNum  int
}

// LineType indicates whether a line was added, removed, or is context
type LineType int

const (
	Context LineType = iota
	Addition
	Deletion
)

// DiffResult contains the complete diff information
type DiffResult struct {
	Hunks []Hunk
	Stats struct {
		Additions int
		Deletions int
		Changes   int
	}
}

// Hunk represents a continuous section of changes
type Hunk struct {
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Lines    []Line
}

// Engine provides diffing capabilities
type Engine struct {
	contextLines int
}

// NewEngine creates a new diff engine with specified context lines
func NewEngine(contextLines int) *Engine {
	return &Engine{
		contextLines: contextLines,
	}
}

// Diff generates a line-by-line diff between two contents
func (e *Engine) Diff(oldContent, newContent []byte) (*DiffResult, error) {
	oldLines := bytes.Split(bytes.TrimSuffix(oldContent, []byte{'\n'}), []byte{'\n'})
	newLines := bytes.Split(bytes.TrimSuffix(newContent, []byte{'\n'}), []byte{'\n'})

	result := &DiffResult{}
	
	// Generate LCS (Longest Common Subsequence) matrix
	lcs := e.computeLCS(oldLines, newLines)
	
	// Extract hunks from LCS
	hunks := e.extractHunks(oldLines, newLines, lcs)
	
	// Add context lines
	result.Hunks = e.addContextLines(hunks, oldLines, newLines)
	
	// Calculate stats
	for _, hunk := range result.Hunks {
		for _, line := range hunk.Lines {
			switch line.Type {
			case Addition:
				result.Stats.Additions++
			case Deletion:
				result.Stats.Deletions++
			}
		}
	}
	result.Stats.Changes = result.Stats.Additions + result.Stats.Deletions

	return result, nil
}

// computeLCS creates a matrix for longest common subsequence
func (e *Engine) computeLCS(oldLines, newLines [][]byte) [][]int {
	matrix := make([][]int, len(oldLines)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(newLines)+1)
	}

	for i := 1; i <= len(oldLines); i++ {
		for j := 1; j <= len(newLines); j++ {
			if bytes.Equal(oldLines[i-1], newLines[j-1]) {
				matrix[i][j] = matrix[i-1][j-1] + 1
			} else {
				matrix[i][j] = max(matrix[i-1][j], matrix[i][j-1])
			}
		}
	}

	return matrix
}

// extractHunks generates diff hunks from the LCS matrix
func (e *Engine) extractHunks(oldLines, newLines [][]byte, lcs [][]int) []Hunk {
	var hunks []Hunk
	var currentHunk *Hunk

	i, j := len(oldLines), len(newLines)
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && bytes.Equal(oldLines[i-1], newLines[j-1]) {
			if currentHunk != nil {
				currentHunk.Lines = append([]Line{{
					Type:    Context,
					Content: string(oldLines[i-1]),
				}}, currentHunk.Lines...)
			}
			i--
			j--
		} else if j > 0 && (i == 0 || lcs[i][j-1] >= lcs[i-1][j]) {
			if currentHunk == nil {
				currentHunk = &Hunk{
					OldStart: i,
					NewStart: j,
				}
			}
			currentHunk.Lines = append([]Line{{
				Type:    Addition,
				Content: string(newLines[j-1]),
			}}, currentHunk.Lines...)
			currentHunk.NewLines++
			j--
		} else if i > 0 && (j == 0 || lcs[i][j-1] < lcs[i-1][j]) {
			if currentHunk == nil {
				currentHunk = &Hunk{
					OldStart: i,
					NewStart: j,
				}
			}
			currentHunk.Lines = append([]Line{{
				Type:    Deletion,
				Content: string(oldLines[i-1]),
			}}, currentHunk.Lines...)
			currentHunk.OldLines++
			i--
		}

		if currentHunk != nil && len(currentHunk.Lines) > 0 {
			hunks = append([]Hunk{*currentHunk}, hunks...)
			currentHunk = nil
		}
	}

	return hunks
}

// addContextLines adds surrounding context to hunks
func (e *Engine) addContextLines(hunks []Hunk, oldLines, _ [][]byte) []Hunk {
	if e.contextLines == 0 {
		return hunks
	}

	var result []Hunk
	for i, hunk := range hunks {
		// Add preceding context
		contextStart := max(0, hunk.OldStart-e.contextLines)
		for j := contextStart; j < hunk.OldStart; j++ {
			hunk.Lines = append([]Line{{
				Type:    Context,
				Content: string(oldLines[j]),
			}}, hunk.Lines...)
		}

		// Add following context
		if i < len(hunks)-1 {
			contextEnd := min(len(oldLines), hunk.OldStart+hunk.OldLines+e.contextLines)
			for j := hunk.OldStart + hunk.OldLines; j < contextEnd; j++ {
				hunk.Lines = append(hunk.Lines, Line{
					Type:    Context,
					Content: string(oldLines[j]),
				})
			}
		}

		result = append(result, hunk)
	}

	return result
}

// Format returns a string representation of the diff
func (r *DiffResult) Format() string {
	var buf bytes.Buffer

	for _, hunk := range r.Hunks {
		fmt.Fprintf(&buf, "@@ -%d,%d +%d,%d @@\n",
			hunk.OldStart, hunk.OldLines,
			hunk.NewStart, hunk.NewLines)

		for _, line := range hunk.Lines {
			switch line.Type {
			case Addition:
				buf.WriteString("+ ")
			case Deletion:
				buf.WriteString("- ")
			case Context:
				buf.WriteString("  ")
			}
			buf.WriteString(line.Content)
			buf.WriteString("\n")
		}
	}

	return buf.String()
}