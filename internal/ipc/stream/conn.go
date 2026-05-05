package stream

import (
	"io"
	"sync"
	"time"
)

// Conn ties framing and codec together.
type Conn struct {
	framer  *Framer
	codec   Codec
	writeMu sync.Mutex
}

func NewConn(r io.Reader, w io.Writer, codec Codec, maxSize int) *Conn {
	if codec == nil {
		codec = JSONCodec{}
	}
	return &Conn{framer: NewFramer(r, w, maxSize), codec: codec}
}

func (c *Conn) ReadEnvelope() (Envelope, error) {
	frame, err := c.framer.ReadFrame()
	if err != nil {
		return Envelope{}, err
	}
	var env Envelope
	if err := c.codec.Unmarshal(frame, &env); err != nil {
		return Envelope{}, err
	}
	return env, nil
}

func (c *Conn) WriteEnvelope(env Envelope) error {
	if env.Version == 0 {
		env.Version = CurrentVersion
	}
	if env.Timestamp == 0 {
		env.Timestamp = time.Now().UnixNano()
	}
	payload, err := c.codec.Marshal(env)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.framer.WriteFrame(payload)
}
