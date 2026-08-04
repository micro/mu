package prayer

import "math"

// Kaaba coordinates in Mecca — the point every qibla bearing is measured to.
const (
	kaabaLat = 21.4225
	kaabaLon = 39.8262
)

// earthRadiusKm is the mean radius, good enough for a "how far is Mecca" figure.
const earthRadiusKm = 6371.0

// QiblaBearing returns the compass bearing in degrees from true north (0–360)
// that points to the Kaaba from the given location. It is the initial bearing
// of the great-circle path, which is what a qibla compass shows.
func QiblaBearing(lat, lon float64) float64 {
	φ1 := rad(lat)
	φ2 := rad(kaabaLat)
	Δλ := rad(kaabaLon - lon)

	y := math.Sin(Δλ) * math.Cos(φ2)
	x := math.Cos(φ1)*math.Sin(φ2) - math.Sin(φ1)*math.Cos(φ2)*math.Cos(Δλ)

	deg := math.Atan2(y, x) * 180 / math.Pi
	return math.Mod(deg+360, 360)
}

// DistanceToMeccaKm returns the great-circle distance to the Kaaba in km.
func DistanceToMeccaKm(lat, lon float64) float64 {
	φ1, φ2 := rad(lat), rad(kaabaLat)
	Δφ := rad(kaabaLat - lat)
	Δλ := rad(kaabaLon - lon)

	a := math.Sin(Δφ/2)*math.Sin(Δφ/2) +
		math.Cos(φ1)*math.Cos(φ2)*math.Sin(Δλ/2)*math.Sin(Δλ/2)
	return earthRadiusKm * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// CompassPoint names a bearing: 42° -> "NE". Sixteen points, which is as
// precise as a named direction usefully gets.
func CompassPoint(bearing float64) string {
	points := []string{
		"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE",
		"S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW",
	}
	i := int(math.Mod(bearing/22.5+0.5, 16))
	if i < 0 {
		i += 16
	}
	return points[i]
}

func rad(d float64) float64 { return d * math.Pi / 180 }
