package adsb

import (
	"math"
	"testing"
)

func TestCprBase(t *testing.T) {
	if got := cprBase(false); got != 360.0 {
		t.Errorf("cprBase(false) = %f, want 360.0", got)
	}
	if got := cprBase(true); got != 90.0 {
		t.Errorf("cprBase(true) = %f, want 90.0", got)
	}
}

func TestMod(t *testing.T) {
	tests := []struct {
		a, b, want float64
	}{
		{10, 3, 1},
		{-10, 3, 2},
		{7, 7, 0},
		{0, 5, 0},
		{359, 360, 359},
	}
	for _, tt := range tests {
		got := mod(tt.a, tt.b)
		if math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("mod(%f, %f) = %f, want %f", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCprNL(t *testing.T) {
	tests := []struct {
		lat  float64
		want uint8
	}{
		{0, 59},
		{-0.1, 59},
		{88, 1},
		{52.0, 36},  // Amsterdam area
		{-52.0, 36}, // symmetric
	}
	for _, tt := range tests {
		got := cprNL(tt.lat)
		if got != tt.want {
			t.Errorf("cprNL(%f) = %d, want %d", tt.lat, got, tt.want)
		}
	}
}

// Test vectors extracted from hex messages in message_test.go via Message.CPR().
var (
	// From "8da9450d60bde138e8638c939134" (TC=12, local decode ref=[52.258, 3.919])
	airborneLocal = &CPR{Nb: 17, F: 0, Lat: 40052, Lon: 25484}

	// From "8da8028758ab0028de078689d437" (TC=11, even) and
	// "8da8028758ab07b0b8876e81eb25" (TC=11, odd), global decode.
	airborneGlobalEven = &CPR{Nb: 17, F: 0, Lat: 5231, Lon: 1926}
	airborneGlobalOdd  = &CPR{Nb: 17, F: 1, Lat: 120924, Lon: 34670}

	// From "8D4841FF380002A3D98000000000" (TC=7, even) and
	// "8D4841FF38000452F7676C000000" (TC=7, odd), surface near Amsterdam Schiphol.
	surfaceEven = &CPR{Nb: 17, F: 0, Lat: 86508, Lon: 98304, Surface: true}
	surfaceOdd  = &CPR{Nb: 17, F: 1, Lat: 10619, Lon: 92012, Surface: true}
)

func TestDecodeLocal_Airborne(t *testing.T) {
	ref := []float64{43.14, -89.33}
	pos, err := airborneLocal.DecodeLocal(ref)
	if err != nil {
		t.Fatalf("DecodeLocal airborne: %v", err)
	}
	// Expected values from message_test.go testDF17PosLocal.
	if math.Abs(pos[0]-43.83300781) > 0.001 {
		t.Errorf("lat = %f, want ~43.833", pos[0])
	}
	if math.Abs(pos[1]-(-90.46484375)) > 0.001 {
		t.Errorf("lon = %f, want ~-90.465", pos[1])
	}
}

func TestDecodeLocal_Surface(t *testing.T) {
	ref := []float64{51.99, 4.375}
	pos, err := surfaceEven.DecodeLocal(ref)
	if err != nil {
		t.Fatalf("DecodeLocal surface: %v", err)
	}
	if math.Abs(pos[0]-51.99009704589844) > 0.0001 {
		t.Errorf("lat = %f, want ~51.9901", pos[0])
	}
	if math.Abs(pos[1]-4.375) > 0.0001 {
		t.Errorf("lon = %f, want ~4.375", pos[1])
	}
}

func TestDecodeLocal_Errors(t *testing.T) {
	c := &CPR{Nb: 17, F: 0, Lat: 1000, Lon: 1000}

	if _, err := c.DecodeLocal([]float64{0}); err == nil {
		t.Error("expected error for wrong slice length")
	}
	if _, err := c.DecodeLocal([]float64{91, 0}); err == nil {
		t.Error("expected error for lat > 90")
	}
	if _, err := c.DecodeLocal([]float64{0, 191}); err == nil {
		t.Error("expected error for lon > 190")
	}
}

func TestDecodeGlobalPosition_Airborne(t *testing.T) {
	pos, err := DecodeGlobalPosition(airborneGlobalEven, airborneGlobalOdd)
	if err != nil {
		t.Fatalf("DecodeGlobalPosition airborne: %v", err)
	}
	// Expected values from message_test.go testDF17PosGlobal.
	if math.Abs(pos[0]-42.23945229) > 0.001 {
		t.Errorf("lat = %f, want ~42.2395", pos[0])
	}
	if math.Abs(pos[1]-(-89.87851165)) > 0.001 {
		t.Errorf("lon = %f, want ~-89.8785", pos[1])
	}
}

func TestDecodeGlobalPosition_Surface_Rejected(t *testing.T) {
	if _, err := DecodeGlobalPosition(surfaceEven, surfaceOdd); err == nil {
		t.Error("expected error when calling DecodeGlobalPosition with surface CPR")
	}
}

func TestDecodeGlobalSurfacePosition(t *testing.T) {
	// Reference near Amsterdam Schiphol
	pos, err := DecodeGlobalSurfacePosition(surfaceEven, surfaceOdd, 52.0, 4.5)
	if err != nil {
		t.Fatalf("DecodeGlobalSurfacePosition: %v", err)
	}
	if math.Abs(pos[0]-51.99) > 0.01 {
		t.Errorf("lat = %f, want ~51.99", pos[0])
	}
	if math.Abs(pos[1]-4.375) > 0.01 {
		t.Errorf("lon = %f, want ~4.375", pos[1])
	}
}

func TestDecodeGlobalSurfacePosition_SouthernHemisphere(t *testing.T) {
	// Same CPR data but reference in southern hemisphere should select that quadrant
	pos, err := DecodeGlobalSurfacePosition(surfaceEven, surfaceOdd, -38.0, 4.5)
	if err != nil {
		t.Fatalf("DecodeGlobalSurfacePosition southern: %v", err)
	}
	// Should be ~51.99-90 = ~-38.01
	if pos[0] > 0 || math.Abs(pos[0]-(-38.01)) > 0.1 {
		t.Errorf("lat = %f, want negative (southern hemisphere)", pos[0])
	}
}

func TestDecodeGlobalSurfacePosition_LonDisambiguation(t *testing.T) {
	// Reference with longitude offset by 90° should select different quadrant
	pos1, err := DecodeGlobalSurfacePosition(surfaceEven, surfaceOdd, 52.0, 4.5)
	if err != nil {
		t.Fatalf("DecodeGlobalSurfacePosition ref1: %v", err)
	}
	pos2, err := DecodeGlobalSurfacePosition(surfaceEven, surfaceOdd, 52.0, 94.5)
	if err != nil {
		t.Fatalf("DecodeGlobalSurfacePosition ref2: %v", err)
	}
	lonDiff := math.Abs(pos2[1] - pos1[1])
	if math.Abs(lonDiff-90) > 1 {
		t.Errorf("longitude quadrant shift = %f, want ~90", lonDiff)
	}
}

func TestDecodeGlobalSurfacePosition_AirborneRejected(t *testing.T) {
	if _, err := DecodeGlobalSurfacePosition(airborneGlobalEven, airborneGlobalOdd, 42.0, -90.0); err == nil {
		t.Error("expected error when calling DecodeGlobalSurfacePosition with airborne CPR")
	}
}

func TestDecodeGlobalPosition_Errors(t *testing.T) {
	t.Run("nil_argument", func(t *testing.T) {
		if _, err := DecodeGlobalPosition(nil, airborneGlobalOdd); err == nil {
			t.Error("expected error for nil c1")
		}
	})

	t.Run("mismatched_nb", func(t *testing.T) {
		c := &CPR{Nb: 12, F: 0, Lat: 1, Lon: 1}
		if _, err := DecodeGlobalPosition(c, airborneGlobalOdd); err == nil {
			t.Error("expected error for mismatched Nb")
		}
	})

	t.Run("same_format", func(t *testing.T) {
		c := &CPR{Nb: 17, F: 1, Lat: 1, Lon: 1}
		if _, err := DecodeGlobalPosition(c, airborneGlobalOdd); err == nil {
			t.Error("expected error for same F bit")
		}
	})

	t.Run("mixed_surface", func(t *testing.T) {
		if _, err := DecodeGlobalPosition(airborneGlobalEven, surfaceOdd); err == nil {
			t.Error("expected error for mixed surface/airborne")
		}
	})
}
