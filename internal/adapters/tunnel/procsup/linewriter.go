package procsup

import "bytes"

// maxLine bounds one reported line. A process that prints megabytes without a
// newline must not grow our memory without limit, so such output is cut into
// chunks of this size and reported as several lines.
const maxLine = 16 << 10

// lineWriter turns the bytes a process writes into lines. It is an io.Writer
// handed to exec, which copies the process's output into it from one goroutine,
// so a lineWriter needs no lock; flush is called only after that copy is done.
type lineWriter struct {
	emit func(line string)
	buf  []byte
}

func newLineWriter(emit func(string)) *lineWriter { return &lineWriter{emit: emit} }

// Write implements io.Writer. It never fails: a consumer that cannot keep up
// would otherwise stall the child on a full pipe.
func (w *lineWriter) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			w.buf = append(w.buf, p...)
			for len(w.buf) >= maxLine {
				w.emitLine(w.buf[:maxLine])
				w.buf = w.buf[maxLine:]
			}
			break
		}
		w.buf = append(w.buf, p[:i]...)
		w.emitLine(w.buf)
		w.buf = w.buf[:0]
		p = p[i+1:]
	}
	return n, nil
}

// flush reports a final line that had no newline.
func (w *lineWriter) flush() {
	if len(w.buf) > 0 {
		w.emitLine(w.buf)
		w.buf = nil
	}
}

func (w *lineWriter) emitLine(b []byte) {
	// Windows tools end lines with \r\n, and progress output uses bare \r.
	line := string(bytes.TrimRight(b, "\r"))
	if line != "" {
		w.emit(line)
	}
}
