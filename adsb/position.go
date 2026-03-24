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
	"math"
)

// CPR is an extended squitter compact position report.
type CPR struct {
	Nb      uint8  // number of encoded bits (17, 19, 14 or 12)
	T       uint8  // time bit
	F       uint8  // format bit
	Lat     uint32 // encoded latitude
	Lon     uint32 // encoded longitude
	Surface bool   // true for surface position (TC 5-8), false for airborne (TC 9-18)
}

// DecodeLocal decodes an encoded position to a global latitude and
// longitude by comparing the position to a known reference point.
// Argument and return value is in the format [latitude, longitude].
func (c *CPR) DecodeLocal(rp []float64) ([]float64, error) {
	switch {
	case len(rp) != 2:
		return nil, newError(nil, "must provide [lat, lon] as argument")
	case rp[0] > 90 || rp[0] < -90:
		return nil, newError(nil, "latitude out of range (-90 to 90)")
	case rp[1] > 190 || rp[1] < -180:
		return nil, newError(nil, "longitude out of range (-180 to 180)")
	}

	latr := rp[0]
	lonr := rp[1]
	latc := float64(c.Lat) / 131072
	lonc := float64(c.Lon) / 131072

	base := cprBase(c.Surface)

	dlat := base / float64(60-c.F)

	j := math.Floor(latr/dlat) +
		math.Floor((mod(latr, dlat)/dlat)-latc+0.5)

	coord := make([]float64, 2)

	coord[0] = dlat * (j + latc)

	var dlon float64

	nl := float64(cprNL(coord[0]) - c.F)

	if nl == 0 {
		dlon = base
	} else {
		dlon = base / nl
	}

	m := math.Floor(lonr/dlon) +
		math.Floor((mod(lonr, dlon)/dlon)-lonc+0.5)

	coord[1] = dlon * (m + lonc)

	return coord, nil
}

// DecodeGlobalPosition decodes an encoded position to a globally
// unambiguous latitude and longitude by combining two CPR messages.
// The two messages must have different formats (CPR.F) and must have
// a time difference of less than 10 seconds (3 NM distance). The
// return value is in the format [latitude, longitude].
//
// For surface positions use DecodeGlobalSurfacePosition which
// requires a reference location to resolve the 4-way zone ambiguity.
func DecodeGlobalPosition(c1 *CPR, c2 *CPR) ([]float64, error) {
	if err := validateGlobalPair(c1, c2); err != nil {
		return nil, err
	}
	if c1.Surface {
		return nil, newError(nil, "use DecodeGlobalSurfacePosition for surface CPR")
	}

	lat0, lon0, lat1, lon1, t0 := extractGlobalFields(c1, c2)
	rlat0, rlat1, err := resolveGlobalLatitudes(lat0, lat1, c1.Surface)
	if err != nil {
		return nil, err
	}

	if rlat0 >= 270 {
		rlat0 -= 360
	}
	if rlat1 >= 270 {
		rlat1 -= 360
	}

	if cprNL(rlat0) != cprNL(rlat1) {
		return nil, newError(nil, "positions cross latitude boundary")
	}

	coord := calcGlobal(t0, lon0, lon1, rlat0, rlat1, false)
	return coord, nil
}

// DecodeGlobalSurfacePosition decodes a surface CPR position using two
// frames and a reference location for zone disambiguation. Surface CPR
// uses 90° zones creating a 4-way ambiguity that requires the reference
// to resolve (readsb cpr.c:decodeCPRsurface).
func DecodeGlobalSurfacePosition(c1, c2 *CPR, refLat, refLon float64) ([]float64, error) {
	if err := validateGlobalPair(c1, c2); err != nil {
		return nil, err
	}
	if !c1.Surface {
		return nil, newError(nil, "use DecodeGlobalPosition for airborne CPR")
	}

	lat0, lon0, lat1, lon1, t0 := extractGlobalFields(c1, c2)
	rlat0, rlat1, err := resolveGlobalLatitudes(lat0, lat1, true)
	if err != nil {
		return nil, err
	}

	// Latitude disambiguation: surface CPR produces rlat in [0, 90).
	// Pick the hemisphere closest to the reference latitude.
	// Only two valid quadrants exist for latitude: [-90,0] and [0,90].
	rlat0 = disambiguateSurfaceLatitude(rlat0, refLat)
	rlat1 = disambiguateSurfaceLatitude(rlat1, refLat)

	if rlat0 < -90 || rlat0 > 90 || rlat1 < -90 || rlat1 > 90 {
		return nil, newError(nil, "latitude out of range after disambiguation")
	}

	if cprNL(rlat0) != cprNL(rlat1) {
		return nil, newError(nil, "positions cross latitude boundary")
	}

	coord := calcGlobal(t0, lon0, lon1, rlat0, rlat1, true)

	// Longitude disambiguation: all four 90° quadrants are valid.
	// Shift to the quadrant closest to the reference longitude.
	coord[1] += math.Floor((refLon-coord[1]+45)/90) * 90
	// Normalize to [-180, 180)
	coord[1] -= math.Floor((coord[1]+180)/360) * 360

	return coord, nil
}

func validateGlobalPair(c1, c2 *CPR) error {
	switch {
	case c1 == nil || c2 == nil:
		return newError(nil, "incomplete arguments")
	case c1.Nb != c2.Nb:
		return newError(nil, "bit encoding must be equal")
	case c1.F == c2.F:
		return newError(nil, "format must be different")
	case c1.Surface != c2.Surface:
		return newError(nil, "surface flag must be equal")
	}
	return nil
}

func extractGlobalFields(c1, c2 *CPR) (lat0, lon0, lat1, lon1 float64, t0 bool) {
	if c1.F == 0 {
		t0 = false
		lat0 = float64(c1.Lat) / 131072
		lon0 = float64(c1.Lon) / 131072
		lat1 = float64(c2.Lat) / 131072
		lon1 = float64(c2.Lon) / 131072
	} else {
		t0 = true
		lat0 = float64(c2.Lat) / 131072
		lon0 = float64(c2.Lon) / 131072
		lat1 = float64(c1.Lat) / 131072
		lon1 = float64(c1.Lon) / 131072
	}
	return
}

func resolveGlobalLatitudes(lat0, lat1 float64, surface bool) (rlat0, rlat1 float64, err error) {
	base := cprBase(surface)
	dlat0 := base / 60.0
	dlat1 := base / 59.0

	j := math.Floor(((59 * lat0) - (60 * lat1)) + 0.5)

	rlat0 = dlat0 * (mod(j, 60) + lat0)
	rlat1 = dlat1 * (mod(j, 59) + lat1)
	return rlat0, rlat1, nil
}

// disambiguateSurfaceLatitude picks the hemisphere closest to refLat.
// rlat is in [0, 90); the two candidates are rlat and rlat-90.
func disambiguateSurfaceLatitude(rlat, refLat float64) float64 {
	if rlat == 0 {
		if refLat < -45 {
			return -90
		}
		if refLat > 45 {
			return 90
		}
		return 0
	}
	if (rlat - refLat) > 45 {
		return rlat - 90
	}
	return rlat
}

func calcGlobal(t0 bool, lon0, lon1, rlat0, rlat1 float64, surface bool) []float64 {
	var nl, ni, dlon, lonc float64

	base := cprBase(surface)

	coord := make([]float64, 2)

	if t0 { //nolint:nestif // variables assigned based on t0 type
		coord[0] = rlat0
		nl = float64(cprNL(rlat0))

		if nl <= 1 {
			ni = 1
		} else {
			ni = nl
		}

		dlon = base / ni
		lonc = lon0
	} else {
		coord[0] = rlat1
		nl = float64(cprNL(rlat1))

		if nl-1 <= 1 {
			ni = 1
		} else {
			ni = nl - 1
		}

		dlon = base / ni
		lonc = lon1
	}

	m := math.Floor(((lon0 * (nl - 1)) - (lon1 * nl)) + 0.5)

	coord[1] = dlon * (mod(m, ni) + lonc)
	if coord[1] >= 180 {
		coord[1] -= 360
	}

	return coord
}

// cprBase returns 90° for surface CPR (TC 5-8) or 360° for airborne (TC 9-18).
// Surface CPR uses 90° zones giving 4x finer resolution for the same
// 17-bit encoding (ICAO Doc 9871 / DO-260B).
func cprBase(surface bool) float64 {
	if surface {
		return 90.0
	}
	return 360.0
}

// mod implements the MOD function as defined in the ADS-B
// specifications.
func mod(a float64, b float64) float64 {
	return a - (b * math.Floor(a/b))
}

/* Lookup table computed with the following code:
tbl := make(map[int]float64)

for nl := 59; nl > 1; nl-- {
	a := 1 - math.Cos(math.Pi/30)
	b := 1 - math.Cos(2*math.Pi/float64(nl))
	c := math.Sqrt(a / b)

	tbl[nl] = (180 / math.Pi) * math.Acos(c)

	fmt.Printf("%d: %s\n", nl, big.NewFloat(tbl[nl]).String())
}
*/

var nlTbl = map[uint8]float64{
	59: 10.4704713,
	58: 14.82817437,
	57: 18.18626357,
	56: 21.02939493,
	55: 23.54504487,
	54: 25.82924707,
	53: 27.9389871,
	52: 29.91135686,
	51: 31.77209708,
	50: 33.53993436,
	49: 35.22899598,
	48: 36.85025108,
	47: 38.41241892,
	46: 39.92256684,
	45: 41.38651832,
	44: 42.80914012,
	43: 44.19454951,
	42: 45.54626723,
	41: 46.86733252,
	40: 48.16039128,
	39: 49.42776439,
	38: 50.67150166,
	37: 51.89342469,
	36: 53.09516153,
	35: 54.27817472,
	34: 55.44378444,
	33: 56.59318756,
	32: 57.72747354,
	31: 58.84763776,
	30: 59.95459277,
	29: 61.04917774,
	28: 62.13216659,
	27: 63.20427479,
	26: 64.26616523,
	25: 65.3184531,
	24: 66.36171008,
	23: 67.39646774,
	22: 68.42322022,
	21: 69.44242631,
	20: 70.45451075,
	19: 71.45986473,
	18: 72.45884545,
	17: 73.45177442,
	16: 74.43893416,
	15: 75.42056257,
	14: 76.39684391,
	13: 77.36789461,
	12: 78.33374083,
	11: 79.29428225,
	10: 80.24923213,
	9:  81.19801349,
	8:  82.13956981,
	7:  83.07199445,
	6:  83.99173563,
	5:  84.89166191,
	4:  85.75541621,
	3:  86.53536998,
	2:  87,
}

// cprNL implements the longitude zone lookup table.
func cprNL(x float64) uint8 {
	x = math.Abs(x)

	var i uint8

	for i = 59; i > 1; i-- {
		if x < nlTbl[i] {
			return i
		}
	}

	return 1
}
