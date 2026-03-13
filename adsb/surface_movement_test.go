package adsb

import (
	"math"
	"testing"
)

func TestDecodeMovementV0(t *testing.T) {
	tests := []struct {
		movement uint8
		want     float64
	}{
		{0, 0},       // no data
		{1, 0},       // stopped
		{2, 0.1875},  // 0.125 + (0+0.5)*0.125
		{8, 0.9375},  // 0.125 + (6+0.5)*0.125
		{9, 1.125},   // 1 + (0+0.5)*0.25
		{12, 1.875},  // 1 + (3+0.5)*0.25
		{13, 2.25},   // 2 + (0+0.5)*0.50
		{38, 14.75},  // 2 + (25+0.5)*0.50
		{39, 15.5},   // 15 + (0+0.5)*1
		{93, 69.5},   // 15 + (54+0.5)*1
		{94, 71},     // 70 + (0+0.5)*2
		{108, 99},    // 70 + (14+0.5)*2
		{109, 102.5}, // 100 + (0+0.5)*5
		{123, 172.5}, // 100 + (14+0.5)*5
		{124, 180},   // > 175kt
		{125, 0},     // invalid
		{126, 0},     // invalid
		{127, 0},     // invalid
	}
	for _, tt := range tests {
		got := decodeMovementV0(tt.movement)
		if math.Abs(got-tt.want) > 0.001 {
			t.Errorf("decodeMovementV0(%d) = %f, want %f", tt.movement, got, tt.want)
		}
	}
}

func TestDecodeMovementV2(t *testing.T) {
	tests := []struct {
		movement uint8
		want     float64
	}{
		{0, 0},
		{1, 0},
		{2, 0.0625}, // 0.125/2
		{3, 0.125 + 0.5*0.875/6},
		{8, 0.125 + 5.5*0.875/6},
		{9, 1.125},
		{13, 2.25},
		{39, 15.5},
		{94, 71},
		{109, 102.5},
		{124, 180},
		{125, 0},
	}
	for _, tt := range tests {
		got := decodeMovementV2(tt.movement)
		if math.Abs(got-tt.want) > 0.001 {
			t.Errorf("decodeMovementV2(%d) = %f, want %f", tt.movement, got, tt.want)
		}
	}
}

func TestDecodeMovementV0V2_IdenticalAboveCode8(t *testing.T) {
	for m := uint8(9); m <= 124; m++ {
		v0 := decodeMovementV0(m)
		v2 := decodeMovementV2(m)
		if math.Abs(v0-v2) > 0.001 {
			t.Errorf("movement=%d: v0=%f != v2=%f", m, v0, v2)
		}
	}
}
