package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// publishedURLLine is one row in the Published URLs footer.
type publishedURLLine struct {
	Label string
	URL   string
}

// serviceTableRow is one row for kaas services list.
type serviceTableRow struct {
	Sandbox  string
	Service  string
	Hostname string
	URL      string
	Guest    int
	Host     int
	Status   string
}

// writePublishedURLsFooter renders a terminal-friendly Published URLs block.
// Fallbacks (in order):
//  1. Callers using JSONL skip this and emit structured events instead.
//  2. NO_COLOR, TERM=dumb, or non-color profile → plain ASCII table.
//  3. Interactive color TTY → lipgloss panel (not Bubble Tea; output is non-interactive).
func writePublishedURLsFooter(w io.Writer, sandbox string, lines []publishedURLLine, catalog, dnsHint string) {
	if len(lines) == 0 && catalog == "" {
		return
	}
	if preferPlainTerm(w) {
		writePublishedURLsPlain(w, sandbox, lines, catalog, dnsHint)
		return
	}
	writePublishedURLsStyled(w, sandbox, lines, catalog, dnsHint)
}

func writeServicesTable(w io.Writer, rows []serviceTableRow) {
	headers := []string{"SANDBOX", "SERVICE", "HOSTNAME", "URL", "GUEST", "HOST", "STATUS"}
	cells := make([][]string, 0, len(rows))
	for _, row := range rows {
		cells = append(cells, []string{
			row.Sandbox,
			row.Service,
			row.Hostname,
			row.URL,
			fmt.Sprintf("%d", row.Guest),
			fmt.Sprintf("%d", row.Host),
			row.Status,
		})
	}
	writeTable(w, headers, cells, tableOpts{URLCols: []int{3}, StatusCols: []int{6}})
}

func writePublishedURLsPlain(w io.Writer, sandbox string, lines []publishedURLLine, catalog, dnsHint string) {
	fmt.Fprintf(w, "Published URLs (%s)\n", sandbox)
	rows := make([][]string, 0, len(lines)+1)
	for _, line := range lines {
		rows = append(rows, []string{line.Label, line.URL})
	}
	if catalog != "" {
		rows = append(rows, []string{"catalog", catalog})
	}
	writeTablePlain(w, []string{"SERVICE", "URL"}, rows)
	if dnsHint != "" {
		fmt.Fprintf(w, "Hint: %s\n", dnsHint)
	}
}

func writePublishedURLsStyled(w io.Writer, sandbox string, lines []publishedURLLine, catalog, dnsHint string) {
	s := newTermStyles(w)

	maxLabel := len("catalog")
	for _, line := range lines {
		if n := len(line.Label); n > maxLabel {
			maxLabel = n
		}
	}
	labelStyle := s.muted.Width(maxLabel).Align(lipgloss.Right)
	catalogLabel := s.faint.Width(maxLabel).Align(lipgloss.Right).Italic(true)

	var b strings.Builder
	b.WriteString(s.title.Render("Published URLs"))
	b.WriteString("  ")
	b.WriteString(s.faint.Render("sandbox " + sandbox))
	b.WriteString("\n")
	for _, line := range lines {
		b.WriteString(labelStyle.Render(line.Label))
		b.WriteString("  ")
		b.WriteString(s.url.Render(line.URL))
		b.WriteString("\n")
	}
	if catalog != "" {
		b.WriteString(catalogLabel.Render("catalog"))
		b.WriteString("  ")
		b.WriteString(s.url.Render(catalog))
		b.WriteString("\n")
	}
	if dnsHint != "" {
		b.WriteString(s.hint.Render("→ " + dnsHint))
		b.WriteString("\n")
	}
	fmt.Fprintln(w, s.border.Render(strings.TrimRight(b.String(), "\n")))
}
