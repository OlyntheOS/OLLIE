package stream

import (
	"encoding/binary"
	"errors"
	"io"
)

const DefaultMaxFrameSize = 8 << 20

var ErrFrameTooLarge = errors.New("frame too large")

// Framer handles length-prefixed frames.
type Framer struct {
	r       io.Reader
	w       io.Writer
	maxSize int
}

func NewFramer(r io.Reader, w io.Writer, maxSize int) *Framer {
	if maxSize <= 0 {
		maxSize = DefaultMaxFrameSize
	}
	return &Framer{r: r, w: w, maxSize: maxSize}
}

func (f *Framer) ReadFrame() ([]byte, error) {
	var sizeBuf [4]byte
	if _, err := io.ReadFull(f.r, sizeBuf[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(sizeBuf[:])
	if size > uint32(f.maxSize) {
		return nil, ErrFrameTooLarge
	}
	frame := make([]byte, size)
	if _, err := io.ReadFull(f.r, frame); err != nil {
		return nil, err
	}
	return frame, nil
}

func (f *Framer) WriteFrame(frame []byte) error {
	if len(frame) > f.maxSize {
		return ErrFrameTooLarge
	}
	var sizeBuf [4]byte
	binary.BigEndian.PutUint32(sizeBuf[:], uint32(len(frame)))
	if err := writeFull(f.w, sizeBuf[:]); err != nil {
		return err
	}
	return writeFull(f.w, frame)
}

func writeFull(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}
