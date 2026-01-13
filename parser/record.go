package parser

import "io"

type readPeeker interface {
	io.Reader
	Peek(n int) ([]byte, error)
}

type recordingReader struct {
	r   readPeeker
	buf []byte
}

var _ readPeeker = &recordingReader{}

func (r *recordingReader) Read(p []byte) (n int, err error) {
	n, err = r.r.Read(p)
	r.buf = append(r.buf, p[:n]...)
	return n, err
}

func (r *recordingReader) Peek(n int) ([]byte, error) {
	return r.r.Peek(n)
}
