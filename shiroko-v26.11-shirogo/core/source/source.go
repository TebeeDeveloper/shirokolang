package source

import "strings"

// FileSpan maps a range of lines in the merged buffer back to the
// original source file that produced them. StartLine and EndLine are
// 1-based and inclusive, and refer to positions in the merged buffer.
//
// The span covers only the file's body (post header-stripping), so
// the first body line of a file is StartLine. Original-file line
// numbers are `bufferLine - StartLine + 1`.
type FileSpan struct {
	Path      string
	StartLine int
	EndLine   int
}

type Source struct {
	Lines []string
	Files []FileSpan
}

func New(src string) *Source {
	return &Source{Lines: strings.Split(src, "\n")}
}

// NewWithFiles is the constructor the loader path uses. The `files`
// slice is the span table produced by pkg.Loader; it lets error
// printers name the original source file instead of the merged
// buffer.
func NewWithFiles(src string, files []FileSpan) *Source {
	return &Source{
		Lines: strings.Split(src, "\n"),
		Files: files,
	}
}

// FileAt returns the span that contains the given 1-based buffer
// line, or nil if the line isn't inside any file body (e.g. the
// synthesized header). Nil-safe.
func (s *Source) FileAt(bufferLine int) *FileSpan {
	if s == nil {
		return nil
	}
	for i := range s.Files {
		f := &s.Files[i]
		if bufferLine >= f.StartLine && bufferLine <= f.EndLine {
			return f
		}
	}
	return nil
}

// DisplayFile returns the path of the original file that the given
// buffer line belongs to, or "" if it can't be attributed.
func (s *Source) DisplayFile(bufferLine int) string {
	if f := s.FileAt(bufferLine); f != nil {
		return f.Path
	}
	return ""
}

// DisplayLine converts a buffer line to the corresponding 1-based
// line in its original file. Falls back to the input when no span
// covers it.
func (s *Source) DisplayLine(bufferLine int) int {
	if f := s.FileAt(bufferLine); f != nil {
		return bufferLine - f.StartLine + 1
	}
	return bufferLine
}

// Line returns the 1-based line n of the merged buffer, with tabs
// expanded, or "" if out of range. Nil-safe.
func (s *Source) Line(n int) string {
	if s == nil || n < 1 || n > len(s.Lines) {
		return ""
	}
	return strings.ReplaceAll(s.Lines[n-1], "\t", "    ")
}

// Window returns `before` lines above errLine, errLine itself, and
// `after` lines below. Lines here are always buffer lines — the
// printer should keep using the buffer line for this call and only
// use DisplayLine for the label. Clipped at file boundaries.
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
