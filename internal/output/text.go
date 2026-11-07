package output

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// TextItem is an indivisible context item; explanations stay with their item.
type TextItem struct {
	Section, Text string
	Priority      int
}

// WriteTextItems fits whole items, preferring useful actions over static advice.
// The estimate includes the rendered headings, omissions, and size footer.
func WriteTextItems(w io.Writer, header string, items []TextItem, budget int64) {
	selected := make([]bool, len(items))
	order := make([]int, len(items))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return items[order[i]].Priority > items[order[j]].Priority })
	render := func() string {
		var b strings.Builder
		b.WriteString(header)
		section := ""
		omitted := 0
		for i, item := range items {
			if !selected[i] {
				omitted++
				continue
			}
			if section != item.Section {
				fmt.Fprintf(&b, "\n%s:\n", item.Section)
				section = item.Section
			}
			b.WriteString(item.Text)
			b.WriteByte('\n')
		}
		if omitted > 0 {
			fmt.Fprintf(&b, "\nOmitted: %d context items due to budget or item limits.\n", omitted)
		}
		body := b.String()
		used := EstimateText(body)
		for i := 0; i < 10; i++ {
			footer := fmt.Sprintf("\nActual: %d estimated tokens\n", used)
			if budget > 0 && used > budget {
				footer += "Warning: mandatory metadata exceeds requested budget; increase --budget.\n"
			}
			next := EstimateText(body + footer)
			if next == used {
				return body + footer
			}
			used = next
		}
		return body
	}
	for _, i := range order {
		selected[i] = true
		if budget > 0 && EstimateText(render()) > budget {
			selected[i] = false
		}
	}
	fmt.Fprint(w, render())
}
