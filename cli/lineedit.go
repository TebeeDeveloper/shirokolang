package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/term"
)

const (
	kCtrlA     = 1
	kCtrlB     = 2
	kCtrlC     = 3
	kCtrlD     = 4
	kCtrlE     = 5
	kCtrlF     = 6
	kCtrlH     = 8
	kTab       = 9
	kCtrlK     = 11
	kCtrlL     = 12
	kEnter     = 13
	kCtrlN     = 14
	kCtrlP     = 16
	kCtrlU     = 21
	kCtrlW     = 23
	kEsc       = 27
	kBackspace = 127
)

// lineEditor is a minimal readline-style editor built on top of
// golang.org/x/term. It supports cursor movement, history, in-line
// editing, and tab completion. Multi-byte UTF-8 (Vietnamese input)
// works because everything is stored as []rune.
type lineEditor struct {
	in        *bufio.Reader
	out       io.Writer
	history   []string
	histPos   int
	savedBuf  []rune
	savedCur  int
	completer func(word string) []string
}

func newLineEditor(completer func(word string) []string) *lineEditor {
	return &lineEditor{
		in:        bufio.NewReader(os.Stdin),
		out:       os.Stdout,
		completer: completer,
	}
}

// ReadLine shows `prompt` and reads one logical line. The terminal is
// put into raw mode for the duration of the call.
//
// Returns:
//   - (line, nil)   on Enter
//   - ("", nil)     on Ctrl-C (caller should redisplay prompt)
//   - ("", io.EOF)  on Ctrl-D with an empty buffer
func (e *lineEditor) ReadLine(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		// Not a terminal (piped stdin): fall back to plain reads.
		return e.plainRead(prompt)
	}
	defer term.Restore(fd, oldState)

	buf := []rune{}
	cursor := 0
	e.histPos = len(e.history)

	for {
		e.redraw(prompt, buf, cursor)

		r, _, err := e.in.ReadRune()
		if err != nil {
			return "", err
		}

		switch r {
		case kEnter:
			fmt.Fprint(e.out, "\r\n")
			line := string(buf)
			if len(line) > 0 {
				if e.histPos == len(e.history) || e.history[e.histPos] != line {
					e.history = append(e.history, line)
				}
			}
			return line, nil

		case kCtrlC:
			fmt.Fprint(e.out, "^C\r\n")
			return "", nil

		case kCtrlD:
			if len(buf) == 0 {
				fmt.Fprint(e.out, "\r\n")
				return "", io.EOF
			}
			if cursor < len(buf) {
				buf = append(buf[:cursor], buf[cursor+1:]...)
			}

		case kBackspace, kCtrlH:
			if cursor > 0 {
				buf = append(buf[:cursor-1], buf[cursor:]...)
				cursor--
			}

		case kCtrlA:
			cursor = 0
		case kCtrlE:
			cursor = len(buf)
		case kCtrlB:
			if cursor > 0 {
				cursor--
			}
		case kCtrlF:
			if cursor < len(buf) {
				cursor++
			}
		case kCtrlP:
			e.historyPrev(&buf, &cursor)
		case kCtrlN:
			e.historyNext(&buf, &cursor)
		case kCtrlK:
			buf = buf[:cursor]
		case kCtrlU:
			buf = append([]rune{}, buf[cursor:]...)
			cursor = 0
		case kCtrlW:
			start := cursor
			for start > 0 && unicode.IsSpace(buf[start-1]) {
				start--
			}
			for start > 0 && !unicode.IsSpace(buf[start-1]) {
				start--
			}
			buf = append(buf[:start], buf[cursor:]...)
			cursor = start
		case kCtrlL:
			fmt.Fprint(e.out, "\x1b[2J\x1b[H")
		case kTab:
			e.complete(&buf, &cursor)

		case kEsc:
			if err := e.handleEscape(&buf, &cursor); err != nil {
				return "", err
			}

		default:
			if r >= 32 && r != kBackspace {
				buf = append(buf, 0)
				copy(buf[cursor+1:], buf[cursor:])
				buf[cursor] = r
				cursor++
			}
		}
	}
}

// plainRead is the non-tty fallback used when stdin isn't a terminal.
// It just reads one line, no editing.
func (e *lineEditor) plainRead(prompt string) (string, error) {
	fmt.Fprint(e.out, prompt)
	line, err := e.in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")
	if line != "" {
		e.history = append(e.history, line)
	}
	return line, nil
}

// ---------- drawing ----------

// redraw emits the prompt and buffer, then positions the cursor. We
// clear to end-of-line first (\x1b[K) so shrinking the buffer doesn't
// leave stale characters.
func (e *lineEditor) redraw(prompt string, buf []rune, cursor int) {
	fmt.Fprint(e.out, "\r\x1b[K")
	fmt.Fprint(e.out, prompt)
	fmt.Fprint(e.out, string(buf))
	if cursor < len(buf) {
		fmt.Fprintf(e.out, "\x1b[%dD", len(buf)-cursor)
	}
}

// ---------- escape sequences ----------

// handleEscape consumes the rest of a CSI / SS3 sequence after ESC and
// dispatches it. Supported: arrows, Home/End, Delete.
func (e *lineEditor) handleEscape(buf *[]rune, cursor *int) error {
	b1, err := e.in.ReadByte()
	if err != nil {
		return err
	}
	if b1 != '[' && b1 != 'O' {
		return nil
	}

	var params []byte
	for {
		b, err := e.in.ReadByte()
		if err != nil {
			return err
		}
		if b >= '@' && b <= '~' {
			e.dispatchCSI(b, string(params), buf, cursor)
			return nil
		}
		params = append(params, b)
	}
}

func (e *lineEditor) dispatchCSI(final byte, params string, buf *[]rune, cursor *int) {
	switch final {
	case 'A': // Up
		e.historyPrev(buf, cursor)
	case 'B': // Down
		e.historyNext(buf, cursor)
	case 'C': // Right
		if *cursor < len(*buf) {
			*cursor++
		}
	case 'D': // Left
		if *cursor > 0 {
			*cursor--
		}
	case 'H': // Home
		*cursor = 0
	case 'F': // End
		*cursor = len(*buf)
	case '~':
		switch params {
		case "1", "7":
			*cursor = 0
		case "4", "8":
			*cursor = len(*buf)
		case "3": // Delete
			if *cursor < len(*buf) {
				*buf = append((*buf)[:*cursor], (*buf)[*cursor+1:]...)
			}
		}
	}
}

// ---------- history ----------

func (e *lineEditor) historyPrev(buf *[]rune, cursor *int) {
	if e.histPos == 0 {
		return
	}
	if e.histPos == len(e.history) {
		e.savedBuf = append([]rune{}, *buf...)
		e.savedCur = *cursor
	}
	e.histPos--
	*buf = []rune(e.history[e.histPos])
	*cursor = len(*buf)
}

func (e *lineEditor) historyNext(buf *[]rune, cursor *int) {
	if e.histPos >= len(e.history) {
		return
	}
	e.histPos++
	if e.histPos == len(e.history) {
		*buf = append([]rune{}, e.savedBuf...)
		*cursor = e.savedCur
	} else {
		*buf = []rune(e.history[e.histPos])
		*cursor = len(*buf)
	}
}

// ---------- completion ----------

// complete replaces the word at the cursor with the unique completion,
// extends to the common prefix of many completions, or prints the
// candidate list when nothing can be extended.
func (e *lineEditor) complete(buf *[]rune, cursor *int) {
	start := *cursor
	for start > 0 && isWordChar((*buf)[start-1]) {
		start--
	}
	word := string((*buf)[start:*cursor])
	if e.completer == nil {
		return
	}
	matches := e.completer(word)
	if len(matches) == 0 {
		return
	}

	if len(matches) == 1 {
		e.replaceRange(buf, cursor, start, *cursor, []rune(matches[0]))
		return
	}

	prefix := commonPrefix(matches)
	if len(prefix) > len(word) {
		e.replaceRange(buf, cursor, start, *cursor, []rune(prefix))
	}

	fmt.Fprint(e.out, "\r\n")
	fmt.Fprintln(e.out, strings.Join(matches, "  "))
}

func (e *lineEditor) replaceRange(buf *[]rune, cursor *int, from, to int, repl []rune) {
	newBuf := append([]rune{}, (*buf)[:from]...)
	newBuf = append(newBuf, repl...)
	newBuf = append(newBuf, (*buf)[to:]...)
	*buf = newBuf
	*cursor = from + len(repl)
}

func isWordChar(r rune) bool {
	return r == '_' || r == '.' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func commonPrefix(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	p := xs[0]
	for _, x := range xs[1:] {
		for !strings.HasPrefix(x, p) {
			p = p[:len(p)-1]
			if p == "" {
				return ""
			}
		}
	}
	return p
}

// sortedSet is a tiny helper for dedup + sort used by completers.
func sortedSet(xs []string) []string {
	seen := map[string]bool{}
	out := xs[:0]
	for _, x := range xs {
		if x == "" || seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}
