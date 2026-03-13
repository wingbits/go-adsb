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

package adsb

import (
	"bytes"
	"errors"
)

// Message provides a high-level abstraction for ADS-B messages. The
// methods of Message provide convenient access to common data values.
// Use RawMessage to obtain direct access to the underlying binary data.
type Message struct {
	raw *RawMessage
}

// NewMessage wraps a RawMessage and returns the new Message.
func NewMessage(r *RawMessage) (*Message, error) {
	m := new(Message)
	m.raw = r

	err := m.validateRaw()
	if err != nil {
		return nil, err
	}

	return m, nil
}

// UnmarshalBinary implements the BinaryUnmarshaler interface, storing
// the supplied data in the Message.
//
// If an error is returned that wraps ErrUnsupported, the data was
// successfully Unmarshalled and the Raw() method will still return the
// RawMessage for further inspection.
func (m *Message) UnmarshalBinary(data []byte) error {
	if m.raw == nil {
		m.raw = new(RawMessage)
	}

	err := m.raw.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	return m.validateRaw()
}

// Validate that the downlink format is an expected value.
func (m *Message) validateRaw() error {
	df, err := m.raw.DF()
	if err != nil {
		return err
	}

	switch df {
	case 0, 4, 5, 11, 16, 17, 18, 20, 21, 24:
		return nil
	default:
		return newErrorf(ErrUnsupported, "downlink format %d", df)
	}
}

// Raw returns the underlying RawMessage. The content of the RawMessage
// will be overwritten by a subsequent call to UnmarsahalBinary.
func (m *Message) Raw() *RawMessage {
	return m.raw
}

// ICAO returns the ICAO address as an unsigned integer.
//
// Since the ICAO address is often extracted from the parity field,
// additional validation against a list of known addresses may be
// warranted.
func (m *Message) ICAO() (uint64, error) {
	aa, err := m.raw.AA()
	if err == nil {
		return aa, nil
	} else if !errors.Is(err, ErrNotAvailable) {
		return 0, err
	}

	ap, err := m.raw.AP()
	if err != nil {
		return 0, err
	}

	return ap ^ m.raw.Parity(), nil
}

// Alt returns the altitude.
func (m *Message) Alt() (int64, error) {
	df, err := m.raw.DF()
	if err != nil {
		return 0, newError(err, "error retrieving altitude")
	}

	switch df {
	case 0, 4, 16, 20:
		ac, err := m.raw.AC()
		if err != nil {
			return 0, newError(err, "error retrieving altitude")
		}

		return decodeAC(ac)
	case 17, 18:
		alt, err := m.raw.ESAltitude()
		if err != nil {
			return 0, newError(err, "error retrieving altitude")
		}

		return decodeESAlt(alt)
	default:
		return 0, newError(ErrNotAvailable, "error retrieving altitude")
	}
}

var callChars = []byte(
	"?ABCDEFGHIJKLMNOPQRSTUVWXYZ????? ???????????????0123456789??????")

// Call returns the callsign.
func (m *Message) Call() (string, error) {
	df, err := m.raw.DF()
	if err != nil {
		return "", newError(err, "error retrieving callsign")
	}

	switch df {
	case 17, 18:
		tc, _ := m.raw.ESType()
		if tc < 1 || tc > 4 {
			return "", newError(ErrNotAvailable, "error retrieving callsign")
		}
	case 20, 21:
		if m.raw.Bits(33, 40) != 0x20 {
			return "", newError(ErrNotAvailable, "error retrieving callsign")
		}
	default:
		return "", newError(ErrNotAvailable, "error retrieving callsign")
	}

	bits := m.raw.Bits(41, 88)

	call := make([]byte, 8)

	var i uint
	for i = 0; i < 8; i++ {
		call[i] = callChars[(bits>>(42-(i*6)))&0x3F]
	}

	return string(bytes.TrimRight(call, " ")), nil
}

var sqkTbl = [][]int{
	{25, 23, 21},
	{31, 29, 27},
	{24, 22, 20},
	{32, 30, 28},
}

// Sqk returns the squawk code.
func (m *Message) Sqk() ([]byte, error) {
	sqk := make([]byte, 0, 4)

	df, err := m.raw.DF()
	if err != nil {
		return nil, newError(err, "error retrieving squawk")
	}

	switch df {
	case 5, 21:
	default:
		return nil, newError(ErrNotAvailable, "error retrieving squawk")
	}

	sqk = sqk[0:4]

	for i, v := range sqkTbl {
		for _, x := range v {
			sqk[i] <<= 1
			sqk[i] |= m.raw.Bit(x)
		}
	}

	return sqk, nil
}

// SurfaceMovement holds ground speed and track extracted from surface position messages (TC 5-8).
type SurfaceMovement struct {
	GS_V0      float64 // ground speed in knots, ADS-B v0 decoding
	GS_V2      float64 // ground speed in knots, ADS-B v2 decoding
	GSValid    bool    // true when movement code indicates valid speed data
	Track      float64 // track angle in degrees (0-360)
	TrackValid bool    // true when heading/track status bit is set
}

// SurfaceMovement extracts ground speed and track from a surface position message (TC 5-8).
// Movement is encoded in ME bits 6-12 as a non-linear scale. Track is in ME bits 14-20,
// with a validity bit at ME bit 13.
func (m *Message) SurfaceMovement() (*SurfaceMovement, error) {
	df, err := m.raw.DF()
	if err != nil {
		return nil, newError(err, "error retrieving surface movement")
	}

	switch df {
	case 17, 18:
		tc, err := m.raw.ESType()
		if err != nil {
			return nil, newError(err, "error retrieving surface movement")
		}
		if tc < 5 || tc > 8 {
			return nil, newError(ErrNotAvailable, "not a surface position message")
		}
	default:
		return nil, newError(ErrNotAvailable, "error retrieving surface movement")
	}

	sm := new(SurfaceMovement)

	movement := uint8(m.raw.Bits(38, 44))
	if movement > 0 && movement < 125 {
		sm.GSValid = true
		sm.GS_V0 = decodeMovementV0(movement)
		sm.GS_V2 = decodeMovementV2(movement)
	}

	if m.raw.Bit(45) == 1 {
		sm.TrackValid = true
		sm.Track = float64(m.raw.Bits(46, 52)) * 360.0 / 128.0
	}

	return sm, nil
}

// decodeMovementV2 decodes the 7-bit movement field using ADS-B v2 scale.
// Returns ground speed in knots (midpoint of the encoded range).
func decodeMovementV2(movement uint8) float64 {
	switch {
	case movement >= 125:
		return 0
	case movement == 124:
		return 180
	case movement >= 109:
		return 100 + (float64(movement)-109+0.5)*5
	case movement >= 94:
		return 70 + (float64(movement)-94+0.5)*2
	case movement >= 39:
		return 15 + (float64(movement)-39+0.5)*1
	case movement >= 13:
		return 2 + (float64(movement)-13+0.5)*0.50
	case movement >= 9:
		return 1 + (float64(movement)-9+0.5)*0.25
	case movement >= 3:
		return 0.125 + (float64(movement)-3+0.5)*0.875/6
	case movement >= 2:
		return 0.125 / 2
	default:
		return 0
	}
}

// decodeMovementV0 decodes the 7-bit movement field using ADS-B v0 scale.
// Identical to v2 except for movement codes 2-8 (lowest speed range).
func decodeMovementV0(movement uint8) float64 {
	switch {
	case movement >= 125:
		return 0
	case movement == 124:
		return 180
	case movement >= 109:
		return 100 + (float64(movement)-109+0.5)*5
	case movement >= 94:
		return 70 + (float64(movement)-94+0.5)*2
	case movement >= 39:
		return 15 + (float64(movement)-39+0.5)*1
	case movement >= 13:
		return 2 + (float64(movement)-13+0.5)*0.50
	case movement >= 9:
		return 1 + (float64(movement)-9+0.5)*0.25
	case movement >= 2:
		return 0.125 + (float64(movement)-2+0.5)*0.125
	default:
		return 0
	}
}

// CPR returns the compact position report.
func (m *Message) CPR() (*CPR, error) {
	df, err := m.raw.DF()
	if err != nil {
		return nil, newError(err, "error retrieving position")
	}

	var surface bool

	switch df {
	case 17, 18:
		tc, err := m.raw.ESType()
		if err != nil {
			return nil, newError(err, "error retrieving position")
		}

		switch {
		case tc >= 9 && tc <= 18:
			surface = false
		case tc >= 5 && tc <= 8:
			surface = true
		default:
			return nil, newError(ErrNotAvailable, "error retrieving position")
		}
	default:
		return nil, newError(ErrNotAvailable, "error retrieving position")
	}

	c := new(CPR)
	c.Nb = 17
	c.T = m.raw.Bit(53)
	c.F = m.raw.Bit(54)
	c.Lat = uint32(m.raw.Bits(55, 71))
	c.Lon = uint32(m.raw.Bits(72, 88))
	c.Surface = surface

	return c, nil
}
