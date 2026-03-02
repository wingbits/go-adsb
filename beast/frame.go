// Copyright 2024 Collin Kreklow
//
// Permission is hereby granted, free of charge, to any person obtaining
// a copy of this software and associated documentation files (the
// "Software"), to deal in the Software without restriction, including
// without limitation the rights to use, copy, modify, merge, publish,
// distribute, sublicense, and/or sell copies of the Software, and to
// permit persons to whom the Software is furnished to do so, subject to
// the following conditions:
//
// The above copyright notice and this permission notice shall be
// included in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
// EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
// NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS
// BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN
// ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package beast

import (
	"bytes"
	"time"
)

// Frame is a Beast format message. A Frame is safe to reuse by calling
// UnmarshalBinary with new data.
type Frame struct {
	data bytes.Buffer
}

// UnmarshalBinary stores a Beast message.
func (f *Frame) UnmarshalBinary(data []byte) error {
	f.data.Reset()

	if len(data) < 9 {
		return newError(nil, "received truncated data")
	}

	if !(data[0] == 0x1a &&
		(data[1] == 0x31 || data[1] == 0x32 || data[1] == 0x33 || data[1] == 0x34)) {
		return newErrorf(nil, "invalid data format: %04x", data[0:2])
	}

	for i := 0; i < len(data); i++ {
		if data[i] == 0x1a && (i+1) < len(data) && data[i+1] == 0x1a {
			i++
		}

		f.data.WriteByte(data[i])
	}

	switch data[1] {
	case 0x31:
		if f.data.Len() != 11 {
			return newErrorf(nil, "expected 11 bytes, received %d", f.data.Len())
		}
	case 0x32:
		if f.data.Len() != 16 {
			return newErrorf(nil, "expected 16 bytes, received %d", f.data.Len())
		}
	case 0x33:
		if f.data.Len() != 23 {
			return newErrorf(nil, "expected 23 bytes, received %d", f.data.Len())
		}
	}

	return nil
}

// MarshalBinary returns a Beast message.
func (f *Frame) MarshalBinary() ([]byte, error) {
	if f.data.Len() < 9 {
		return nil, ErrNoData
	}

	ob := bytes.NewBuffer(make([]byte, 0, 25))

	for i, b := range f.data.Bytes() {
		if i > 0 && b == 0x1a {
			ob.WriteByte(0x1a)
		}

		ob.WriteByte(b)
	}

	return ob.Bytes(), nil
}

// Bytes returns the stored frame data. Escaped 0x1a values in the data
// are stripped before storing. To obtain the frame in the escaped wire
// format, use MarshalBinary.
//
// The returned slice remains valid until the next call to
// UnmarshalBinary. Modifying the returned slice directly may impact
// future Frame method calls.
func (f *Frame) Bytes() []byte {
	return f.data.Bytes()
}

// ModeAC returns the Mode AC data in a type 1 frame.
//
// The returned slice remains valid until the next call to
// UnmarshalBinary. Modifying the returned slice directly may impact
// future Frame method calls.
func (f *Frame) ModeAC() ([]byte, error) {
	if f.data.Len() < 10 {
		return nil, ErrNoData
	}

	b := f.data.Bytes()

	if !(b[0] == 0x1a && b[1] == 0x31) {
		return nil, ErrNoData
	}

	return b[9:], nil
}

// ModeS returns the Mode S data in a type 2 or 3 frame.
//
// The returned slice remains valid until the next call to
// UnmarshalBinary. Modifying the returned slice directly may impact
// future Frame method calls.
func (f *Frame) ModeS() ([]byte, error) {
	if f.data.Len() < 10 {
		return nil, ErrNoData
	}

	b := f.data.Bytes()

	if !(b[0] == 0x1a && (b[1] == 0x32 || b[1] == 0x33)) {
		return nil, ErrNoData
	}

	return b[9:], nil
}

// Signal returns the signal level.
func (f *Frame) Signal() (uint8, error) {
	if f.data.Len() < 9 {
		return 0, ErrNoData
	}

	return f.data.Bytes()[8], nil
}

// TimestampTicks returns the raw MLAT timestamp in 12MHz ticks.
func (f *Frame) TimestampTicks() (int64, error) {
	if f.data.Len() < 8 {
		return 0, ErrNoData
	}

	d := f.data.Bytes()
	return int64(d[2])<<40 | int64(d[3])<<32 | int64(d[4])<<24 |
		int64(d[5])<<16 | int64(d[6])<<8 | int64(d[7]), nil
}

// TicksToTimestamp converts 12MHz ticks to a time.Duration.
func TicksToTimestamp(ticks int64) time.Duration {
	return time.Duration(ticks * 1000 / 12).Round(time.Microsecond / 2)
}

// TimestampToTicks converts a time.Duration to 12MHz ticks.
func TimestampToTicks(d time.Duration) int64 {
	return d.Nanoseconds() * 12 / 1000
}

// Timestamp returns the MLAT timestamp as a time.Duration.
func (f *Frame) Timestamp() (time.Duration, error) {
	ticks, err := f.TimestampTicks()
	if err != nil {
		return 0, err
	}
	return TicksToTimestamp(ticks), nil
}

// NewFrame constructs a Frame from individual components.
// The frame type is inferred from the payload length:
//   - 2 bytes → type 0x31 (Mode A/C)
//   - 7 bytes → type 0x32 (Mode S short)
//   - 14 bytes → type 0x33 (Mode S long)
func NewFrame(ticks int64, signal uint8, payload []byte) (*Frame, error) {
	var frameType byte
	switch len(payload) {
	case 2:
		frameType = 0x31
	case 7:
		frameType = 0x32
	case 14:
		frameType = 0x33
	default:
		return nil, newErrorf(nil, "unsupported payload length: %d", len(payload))
	}

	f := &Frame{}
	f.data.Grow(2 + 6 + 1 + len(payload))
	f.data.WriteByte(0x1a)
	f.data.WriteByte(frameType)
	f.data.WriteByte(byte(ticks >> 40))
	f.data.WriteByte(byte(ticks >> 32))
	f.data.WriteByte(byte(ticks >> 24))
	f.data.WriteByte(byte(ticks >> 16))
	f.data.WriteByte(byte(ticks >> 8))
	f.data.WriteByte(byte(ticks))
	f.data.WriteByte(signal)
	f.data.Write(payload)

	return f, nil
}

// Type returns the frame type byte.
func (f *Frame) Type() (byte, error) {
	if f.data.Len() < 10 {
		return 0, ErrNoData
	}

	return f.data.Bytes()[1], nil
}
