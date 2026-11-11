package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"goctx/internal/codexpack"
	"goctx/internal/output"
	"goctx/internal/tokens"
)

type embedInventory struct {
	Declarations          []codexpack.EmbedDeclaration `json:"declarations"`
	OmittedDeclarations   int                          `json:"omitted_declarations"`
	OmittedFiles          int                          `json:"omitted_files"`
	OutputEstimatedTokens int64                        `json:"output_estimated_tokens"`
}

func writeEmbedInventory(w io.Writer, entries []codexpack.EmbedDeclaration, opts output.Options) error {
	totalFiles := 0
	for _, entry := range entries {
		totalFiles += len(entry.Files)
	}
	// Each declaration and each match is a separate item. Only retain
	// complete items, and always retain the declaration for any visible file.
	render := func(items int) []byte {
		report := embedInventory{
			Declarations:        make([]codexpack.EmbedDeclaration, 0),
			OmittedDeclarations: len(entries), OmittedFiles: totalFiles,
		}
		for _, entry := range entries {
			if items == 0 {
				break
			}
			items--
			count := min(items, len(entry.Files))
			entry.Files = entry.Files[:count]
			report.Declarations = append(report.Declarations, entry)
			report.OmittedDeclarations--
			report.OmittedFiles -= count
			items -= count
		}
		for {
			var b bytes.Buffer
			if opts.Format == output.FormatJSON {
				encoder := json.NewEncoder(&b)
				if !opts.CompactJSON {
					encoder.SetIndent("", "  ")
				}
				// The report contains only strings, numbers and slices, all JSON encodable.
				_ = encoder.Encode(report)
			} else {
				if len(entries) == 0 {
					fmt.Fprintln(&b, "No Go embeds found.")
				}
				for _, entry := range report.Declarations {
					fmt.Fprintf(&b, "%s:%d  %s  %q\n", entry.File, entry.Line, entry.Variable, entry.Pattern)
					for _, file := range entry.Files {
						fmt.Fprintf(&b, "  %s (%d estimated file tokens)\n", file.Path, file.EstimatedTokens)
					}
				}
				fmt.Fprintf(&b, "Omitted declarations: %d. Omitted matched files: %d.\nEstimated output tokens: %d\n", report.OmittedDeclarations, report.OmittedFiles, report.OutputEstimatedTokens)
			}
			used := tokens.Estimate(int64(b.Len()))
			if used == report.OutputEstimatedTokens {
				return b.Bytes()
			}
			report.OutputEstimatedTokens = used
		}
	}
	totalItems := len(entries) + totalFiles
	data := render(totalItems)
	if opts.Budget > 0 && tokens.Estimate(int64(len(data))) > opts.Budget {
		minimum := render(0)
		if used := tokens.Estimate(int64(len(minimum))); used > opts.Budget {
			return fmt.Errorf("embed report metadata needs at least %d estimated tokens; increase --budget", used)
		}
		firstTooLarge := sort.Search(totalItems+1, func(items int) bool {
			return tokens.Estimate(int64(len(render(items)))) > opts.Budget
		})
		data = render(firstTooLarge - 1)
	}
	_, err := w.Write(data)
	return err
}
