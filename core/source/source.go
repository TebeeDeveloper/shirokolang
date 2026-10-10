package source

import "strings"

type Source struct {
	Lines []string
}

func New(src string) *Source {
	return &Source{Lines: strings.Split(src, "\n")}
}

// Line returns the 1-based line n, with tabs expanded, or "" if out of
// range. Nil-safe.
func (s *Source) Line(n int) string {
	if s == nil || n < 1 || n > len(s.Lines) {
		return ""
	}
	return strings.ReplaceAll(s.Lines[n-1], "\t", "    ")
}

// Window returns `before` lines above errLine, errLine itself, and
// `after` lines below. Clipped at file boundaries.
func (s *Source) Window(errLine, before, after int) []struct {
	Line int
	Text string
} {
	if s == nil || len(s.Lines) == 0 {
		return nil
	}
	start := errLine - before
	if start < 1 {
		start = 1
	}
	end := errLine + after
	if end > len(s.Lines) {
		end = len(s.Lines)
	}
	var out []struct {
		Line int
		Text string
	}
	for i := start; i <= end; i++ {
		out = append(out, struct {
			Line int
			Text string
		}{i, s.Line(i)})
	}
	return out
}
