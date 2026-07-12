package tools

import (
	"bytes"
	"io"
)

type truncatingBuffer struct {
	buffer    bytes.Buffer
	maximum   int
	truncated bool
}

func newTruncatingBuffer(maximum int) *truncatingBuffer {
	return &truncatingBuffer{maximum: maximum}
}

func (b *truncatingBuffer) Write(value []byte) (int, error) {
	remaining := b.maximum - b.buffer.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(value), nil
	}
	if len(value) > remaining {
		_, _ = b.buffer.Write(value[:remaining])
		b.truncated = true
		return len(value), nil
	}
	return b.buffer.Write(value)
}

func (b *truncatingBuffer) String() string {
	if !b.truncated {
		return b.buffer.String()
	}
	return b.buffer.String() + "\n… output truncated"
}

var _ io.Writer = (*truncatingBuffer)(nil)
