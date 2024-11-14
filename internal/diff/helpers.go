package diff

import "bytes"

func (e *Engine) buildLCSMatrix(oldLines, newLines [][]byte) [][]int {
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

func (e *Engine) generateHunks(oldLines, newLines [][]byte, lcs [][]int) []Hunk {
    var hunks []Hunk
    var currentHunk *Hunk

    i, j := len(oldLines), len(newLines)

    for i > 0 || j > 0 {
        if i > 0 && j > 0 && bytes.Equal(oldLines[i-1], newLines[j-1]) {
            if currentHunk != nil {
                currentHunk.Lines = append([]Line{{
                    Type:    Context,
                    Content: string(oldLines[i-1]),
                    OldNum:  i,
                    NewNum:  j,
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
                hunks = append([]Hunk{*currentHunk}, hunks...)
            }
            currentHunk.Lines = append([]Line{{
                Type:    Addition,
                Content: string(newLines[j-1]),
                NewNum:  j,
            }}, currentHunk.Lines...)
            currentHunk.NewLines++
            j--
        } else if i > 0 {
            if currentHunk == nil {
                currentHunk = &Hunk{
                    OldStart: i,
                    NewStart: j,
                }
                hunks = append([]Hunk{*currentHunk}, hunks...)
            }
            currentHunk.Lines = append([]Line{{
                Type:    Deletion,
                Content: string(oldLines[i-1]),
                OldNum:  i,
            }}, currentHunk.Lines...)
            currentHunk.OldLines++
            i--
        }

        if currentHunk != nil && len(currentHunk.Lines) >= 7 {
            hunks = append([]Hunk{*currentHunk}, hunks...)
            currentHunk = nil
        }
    }

    if currentHunk != nil {
        hunks = append([]Hunk{*currentHunk}, hunks...)
    }

    return hunks
}

func (e *Engine) addContext(hunks []Hunk, oldLines, _ [][]byte) []Hunk {
    if e.contextLines == 0 {
        return hunks
    }

    var result []Hunk

    for i, hunk := range hunks {
        // Add preceding context
        contextStart := max(0, hunk.OldStart-e.contextLines)
        if contextStart < hunk.OldStart {
            newHunk := Hunk{
                OldStart: contextStart,
                NewStart: contextStart,
            }
            for j := contextStart; j < hunk.OldStart; j++ {
                if j < len(oldLines) {
                    newHunk.Lines = append(newHunk.Lines, Line{
                        Type:    Context,
                        Content: string(oldLines[j]),
                        OldNum:  j + 1,
                        NewNum:  j + 1,
                    })
                    newHunk.OldLines++
                    newHunk.NewLines++
                }
            }
            newHunk.Lines = append(newHunk.Lines, hunk.Lines...)
            hunk = newHunk
        }

        // Add following context
        if i < len(hunks)-1 {
            nextHunk := hunks[i+1]
            contextEnd := min(len(oldLines), hunk.OldStart+hunk.OldLines+e.contextLines)
            for j := hunk.OldStart + hunk.OldLines; j < contextEnd && j < nextHunk.OldStart; j++ {
                hunk.Lines = append(hunk.Lines, Line{
                    Type:    Context,
                    Content: string(oldLines[j]),
                    OldNum:  j + 1,
                    NewNum:  j + 1,
                })
                hunk.OldLines++
                hunk.NewLines++
            }
        }

        result = append(result, hunk)
    }

    return result
}

func max(a, b int) int {
    if a > b {
        return a
    }
    return b
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}