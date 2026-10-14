package utils

import (
	"bytes"
	"io"
)

const buildOutputMaxBytes = 1024 * 1024
const buildLineMaxBytes = 64 * 1024
const buildOutputTruncatedMessage = "[Diagnostic output truncated]\n"
const buildLineTruncatedMessage = "[Log line omitted: exceeds 64 KiB limit]\n"

type buildOutputTail struct {
	data      []byte
	truncated bool
}

func (buffer *buildOutputTail) Write(data []byte) (int, error) {
	length := len(data)
	if length == 0 {
		return 0, nil
	}
	limit := buildOutputMaxBytes - len(buildOutputTruncatedMessage)
	if buffer.data == nil {
		buffer.data = make([]byte, 0, limit)
	}
	if length >= limit {
		buffer.truncated = buffer.truncated || len(buffer.data) > 0 || length > limit
		buffer.data = append(buffer.data[:0], data[length-limit:]...)
	} else {
		if overflow := len(buffer.data) + length - limit; overflow > 0 {
			copy(buffer.data, buffer.data[overflow:])
			buffer.data = buffer.data[:len(buffer.data)-overflow]
			buffer.truncated = true
		}
		buffer.data = append(buffer.data, data...)
	}
	return length, nil
}

func (buffer *buildOutputTail) String() string {
	if !buffer.truncated {
		return string(buffer.data)
	}
	if newline := bytes.IndexByte(buffer.data, '\n'); newline >= 0 {
		return buildOutputTruncatedMessage + string(buffer.data[newline+1:])
	}
	return buildOutputTruncatedMessage
}

type buildLineWriter struct {
	writer   io.Writer
	observe  func(string)
	data     []byte
	omitting bool
}

func (writer *buildLineWriter) Write(data []byte) (int, error) {
	length := len(data)
	for len(data) > 0 {
		newline := bytes.IndexByte(data, '\n')
		part := data
		if newline >= 0 {
			part = data[:newline]
		}
		if !writer.omitting {
			if len(writer.data)+len(part) > buildLineMaxBytes {
				writer.data = writer.data[:0]
				writer.omitting = true
			} else {
				writer.data = append(writer.data, part...)
			}
		}
		if newline < 0 {
			break
		}
		if err := writer.Flush(); err != nil {
			return length - len(data), err
		}
		data = data[newline+1:]
	}
	return length, nil
}

func (writer *buildLineWriter) Flush() error {
	data := append(writer.data, '\n')
	if writer.omitting {
		data = []byte(buildLineTruncatedMessage)
	} else if writer.observe != nil {
		writer.observe(string(writer.data))
	}
	written, err := writer.writer.Write(data)
	writer.data = writer.data[:0]
	writer.omitting = false
	if err == nil && written != len(data) {
		return io.ErrShortWrite
	}
	return err
}
