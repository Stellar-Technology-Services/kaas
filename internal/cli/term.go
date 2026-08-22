package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
)

// styleKind selects printStyled emphasis.
type styleKind int

const (
	stylePlain styleKind = iota
	styleSuccess
	styleInfo
	styleWarn
	styleError
)

type termStyles struct {
	title  lipgloss.Style
	header lipgloss.Style
	ok     lipgloss.Style
	warn   lipgloss.Style
	bad    lipgloss.Style
	muted  lipgloss.Style
	url    lipgloss.Style
	hint   lipgloss.Style
	border lipgloss.Style
	faint  lipgloss.Style
}

func newTermStyles(w io.Writer) termStyles {
	r := lipgloss.NewRenderer(w)
	return termStyles{
		title: r.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#5B4B8A", Dark: "#B8A9E8"}),
		header: r.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#5B4B8A", Dark: "#B8A9E8"}),
		ok: r.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#0B6E4F", Dark: "#7DDBB2"}).
			Bold(true),
		warn: r.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#8A6D1D", Dark: "#E6C35C"}),
		bad: r.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#A33B3B", Dark: "#E87B7B"}).
			Bold(true),
		muted: r.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#3D3D3D", Dark: "#A0A0A0"}),
		url: r.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#0B6E4F", Dark: "#7DDBB2"}).
			Bold(true),
		hint: r.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#8A6D1D", Dark: "#E6C35C"}),
		border: r.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#8B7EC8", Dark: "#6C5CE7"}).
			Padding(0, 1).
			MarginTop(1),
		faint: r.NewStyle().Faint(true),
	}
}

func preferPlainTerm(w io.Writer) bool {
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return true
	}
	term := strings.ToLower(strings.TrimSpace(os.Getenv("TERM")))
	if term == "dumb" {
		return true
	}
	return lipgloss.NewRenderer(w).ColorProfile() == termenv.Ascii
}

// tableOpts controls optional per-column styling for writeTable.
type tableOpts struct {
	// StatusCols are 0-based column indexes styled with statusColor.
	StatusCols []int
	// URLCols are 0-based column indexes styled as URLs.
	URLCols []int
}

func writeTable(w io.Writer, headers []string, rows [][]string, opts tableOpts) {
	if preferPlainTerm(w) {
		writeTablePlain(w, headers, rows)
		return
	}
	writeTableStyled(w, headers, rows, opts)
}

func writeTablePlain(w io.Writer, headers []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	_ = tw.Flush()
}

func writeTableStyled(w io.Writer, headers []string, rows [][]string, opts tableOpts) {
	s := newTermStyles(w)
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i := 0; i < len(headers) && i < len(row); i++ {
			if n := visibleLen(row[i]); n > widths[i] {
				widths[i] = n
			}
		}
	}

	statusSet := map[int]struct{}{}
	for _, i := range opts.StatusCols {
		statusSet[i] = struct{}{}
	}
	urlSet := map[int]struct{}{}
	for _, i := range opts.URLCols {
		urlSet[i] = struct{}{}
	}

	var b strings.Builder
	headerParts := make([]string, len(headers))
	for i, h := range headers {
		headerParts[i] = padRight(h, widths[i])
	}
	headerLine := strings.Join(headerParts, "  ")
	b.WriteString(s.header.Render(headerLine))
	b.WriteString("\n")
	b.WriteString(s.faint.Render(strings.Repeat("─", len(headerLine))))
	b.WriteString("\n")
	for _, row := range rows {
		parts := make([]string, len(headers))
		for i := 0; i < len(headers); i++ {
			v := ""
			if i < len(row) {
				v = row[i]
			}
			pad := ""
			if n := widths[i] - visibleLen(v); n > 0 {
				pad = strings.Repeat(" ", n)
			}
			styled := v
			if _, ok := statusSet[i]; ok {
				styled = statusStyle(s, v).Render(v)
			} else if _, ok := urlSet[i]; ok {
				styled = s.url.Render(v)
			}
			parts[i] = styled + pad
		}
		b.WriteString(strings.Join(parts, "  "))
		b.WriteString("\n")
	}
	fmt.Fprint(w, b.String())
}

type kvPair struct {
	Key   string
	Value string
}

func writeKV(w io.Writer, pairs []kvPair) {
	if preferPlainTerm(w) {
		tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		for _, p := range pairs {
			fmt.Fprintf(tw, "%s\t%s\n", p.Key, p.Value)
		}
		_ = tw.Flush()
		return
	}
	s := newTermStyles(w)
	maxKey := 0
	for _, p := range pairs {
		if n := len(p.Key); n > maxKey {
			maxKey = n
		}
	}
	keyStyle := s.muted.Width(maxKey).Align(lipgloss.Left)
	var b strings.Builder
	for _, p := range pairs {
		b.WriteString(keyStyle.Render(p.Key))
		b.WriteString("  ")
		b.WriteString(statusStyle(s, p.Value).Render(p.Value))
		b.WriteString("\n")
	}
	fmt.Fprint(w, b.String())
}

func writeTitle(w io.Writer, title string) {
	if preferPlainTerm(w) {
		fmt.Fprintln(w, title)
		return
	}
	s := newTermStyles(w)
	fmt.Fprintln(w, s.title.Render(title))
}

func writeHint(w io.Writer, hint string) {
	if strings.TrimSpace(hint) == "" {
		return
	}
	if preferPlainTerm(w) {
		fmt.Fprintf(w, "→ %s\n", hint)
		return
	}
	s := newTermStyles(w)
	fmt.Fprintln(w, s.hint.Render("→ "+hint))
}

func writeMuted(w io.Writer, text string) {
	if preferPlainTerm(w) {
		fmt.Fprint(w, text)
		return
	}
	s := newTermStyles(w)
	fmt.Fprint(w, s.muted.Render(strings.TrimRight(text, "\n")))
	if strings.HasSuffix(text, "\n") {
		fmt.Fprintln(w)
	}
}

func statusStyle(s termStyles, value string) lipgloss.Style {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ok", "running", "true", "enabled", "up":
		return s.ok
	case "missing", "failed", "error", "false", "disabled", "down":
		return s.bad
	case "warn", "warning", "stopped", "optional":
		return s.warn
	default:
		return lipgloss.NewStyle()
	}
}

// printStyled writes human prose with optional lipgloss emphasis.
// JSONL mode still emits structured message events.
func printStyled(cmd *cobra.Command, kind styleKind, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if jsonlEnabled(cmd) {
		emitJSONL("message", map[string]any{"command": cmd.CommandPath(), "message": msg})
		return
	}
	w := commandOutput(cmd)
	if preferPlainTerm(w) || kind == stylePlain {
		fmt.Fprint(w, msg)
		return
	}
	s := newTermStyles(w)
	var style lipgloss.Style
	switch kind {
	case styleSuccess:
		style = s.ok
	case styleInfo:
		style = s.title
	case styleWarn:
		style = s.warn
	case styleError:
		style = s.bad
	default:
		fmt.Fprint(w, msg)
		return
	}
	// Style the message body but preserve a trailing newline outside the style.
	trim := strings.TrimRightFunc(msg, unicode.IsSpace)
	suffix := msg[len(trim):]
	fmt.Fprint(w, style.Render(trim)+suffix)
}

func padRight(s string, width int) string {
	if n := width - len(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

func visibleLen(s string) int {
	// Strip common ANSI CSI sequences for width calculation.
	n := 0
	inEsc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inEsc {
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				inEsc = false
			}
			continue
		}
		if c == 0x1b {
			inEsc = true
			continue
		}
		n++
	}
	return n
}
