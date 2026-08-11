// Package flights answers where aircraft are.
//
// Everything here is live position, not schedule. The source is ADS-B: aircraft
// broadcast their own position, a volunteer network of receivers hears them, and
// what comes back is the aeroplane's own account of itself, seconds old. Nothing
// in this package knows what time a flight was meant to leave, and nothing here
// will tell you a departure is delayed — it can only tell you the aeroplane is
// still on the ground, which is the same fact arrived at honestly.
//
// That boundary is the service. Flight status products — FlightAware, Cirium,
// AviationStack — sell the schedule half, and they sell it behind an account and
// a card. The position half is public radio, and the point of putting it here is
// that a caller gets it with neither.
//
// Two words that look interchangeable and are not. A *flight number* is what a
// ticket says, "BA117", built from the airline's two-character IATA code. A
// *callsign* is what the crew says on the radio and what the transponder
// broadcasts, "BAW117", built from the airline's three-letter ICAO code. Usually
// the digits match and the translation is mechanical; sometimes an airline flies
// a number under a different callsign entirely, and then no amount of table
// lookup will find it. Lookup tries the translation, the raw string, and the
// registration, and says plainly when it finds nothing.
package flights

import (
	"fmt"
	"math"
	"strings"

	"mu/internal/app"
	"mu/internal/service"
)

// Load registers the flights service.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("flights", "service register failed: %v", err)
	}
}

// Aircraft is one aeroplane as the network last heard it.
//
// The upstream record carries about sixty fields — navigation modes, signal
// integrity categories, receiver signal strength. This is the handful that
// answers a question somebody actually asked.
type Aircraft struct {
	Hex      string  `json:"hex"`       // ICAO 24-bit address, the aircraft's permanent identity
	Callsign string  `json:"callsign"`  // what the transponder is broadcasting, e.g. "BAW117"
	Reg      string  `json:"reg"`       // registration, e.g. "G-ZBKL"
	Type     string  `json:"type"`      // ICAO type designator, e.g. "B789"
	Lat      float64 `json:"lat"`       //
	Lon      float64 `json:"lon"`       //
	Altitude int     `json:"altitude"`  // feet above mean sea level
	OnGround bool    `json:"on_ground"` //
	Speed    float64 `json:"speed"`     // ground speed in knots
	Track    float64 `json:"track"`     // degrees true, the direction it is actually moving
	Climb    float64 `json:"climb"`     // feet per minute, positive up
	Squawk   string  `json:"squawk"`    // transponder code; 7500/7600/7700 are the emergency ones
	Distance float64 `json:"distance"`  // nautical miles from the point asked about, when there was one
	Bearing  float64 `json:"bearing"`   // degrees from that point
	Age      float64 `json:"age"`       // seconds since the position was heard
}

// Emergency reports whether the transponder is set to one of the three codes
// that mean something is wrong: hijack, radio failure, general emergency.
//
// These are worth surfacing and not worth dramatising. A crew mis-setting a
// digit is far more common than an actual emergency, so the wording says what
// the code means rather than what it might imply.
func (a Aircraft) Emergency() string {
	switch a.Squawk {
	case "7500":
		return "squawking 7500 (unlawful interference)"
	case "7600":
		return "squawking 7600 (radio failure)"
	case "7700":
		return "squawking 7700 (general emergency)"
	}
	return ""
}

// Label is how an aircraft introduces itself: the marketed flight where that can
// be worked out, then the callsign, then the registration, then the hex. Every
// aircraft has the last of those, so this is never empty.
func (a Aircraft) Label() string {
	if a.Callsign != "" {
		if airline, number := SplitCallsign(a.Callsign); airline != nil {
			return fmt.Sprintf("%s %s (%s)", airline.Name, number, a.Callsign)
		}
		return a.Callsign
	}
	if a.Reg != "" {
		return a.Reg
	}
	return a.Hex
}

// Describe renders one aircraft as a line of model-ready text.
func (a Aircraft) Describe(withDistance bool) string {
	var b strings.Builder
	b.WriteString(a.Label())

	if t := AircraftType(a.Type); t != "" {
		b.WriteString(" — " + t)
	} else if a.Type != "" {
		b.WriteString(" — " + a.Type)
	}
	if a.Reg != "" && !strings.Contains(b.String(), a.Reg) {
		b.WriteString(", " + a.Reg)
	}

	switch {
	case a.OnGround:
		b.WriteString(", on the ground")
	case a.Altitude <= 0:
		// Barometric altitude is measured against a pressure setting, not the
		// ground, so an aeroplane on its landing roll routinely broadcasts a
		// small negative number. "-225 ft" reads as a fault; it is an aeroplane
		// on the runway.
		b.WriteString(", at ground level")
	default:
		fmt.Fprintf(&b, ", %s ft", comma(a.Altitude))
		switch {
		case a.Climb > 300:
			fmt.Fprintf(&b, " climbing %s fpm", comma(int(a.Climb)))
		case a.Climb < -300:
			fmt.Fprintf(&b, " descending %s fpm", comma(int(-a.Climb)))
		}
	}
	if a.Speed > 0 {
		fmt.Fprintf(&b, ", %.0f kt", a.Speed)
	}
	if !a.OnGround && a.Speed > 0 {
		fmt.Fprintf(&b, " heading %s", compass(a.Track))
	}
	if withDistance && a.Distance > 0 {
		fmt.Fprintf(&b, ", %.0f nm %s", a.Distance, compass(a.Bearing))
	}
	if e := a.Emergency(); e != "" {
		b.WriteString(" — " + e)
	}
	return b.String()
}

// compass turns a bearing into the eight-point name a person would use. Sixteen
// points would be more precise and less readable, and precision in a bearing is
// not what anyone is asking for when they ask which way a plane is going.
func compass(deg float64) string {
	names := []string{"north", "north-east", "east", "south-east", "south", "south-west", "west", "north-west"}
	for deg < 0 {
		deg += 360
	}
	i := int(math.Mod(deg+22.5, 360) / 45)
	return names[i%8]
}

// comma groups thousands, because 36000 and 3600 are hard to tell apart at a
// glance and altitudes are read at a glance.
func comma(n int) string {
	s := fmt.Sprintf("%d", n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		return "-" + s
	}
	return s
}

// distanceNM is the great-circle distance in nautical miles.
func distanceNM(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 3440.065 // Earth radius in nautical miles
	p1, p2 := lat1*math.Pi/180, lat2*math.Pi/180
	dp := (lat2 - lat1) * math.Pi / 180
	dl := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return r * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// bearingTo is the initial great-circle bearing from one point to another.
func bearingTo(lat1, lon1, lat2, lon2 float64) float64 {
	p1, p2 := lat1*math.Pi/180, lat2*math.Pi/180
	dl := (lon2 - lon1) * math.Pi / 180
	y := math.Sin(dl) * math.Cos(p2)
	x := math.Cos(p1)*math.Sin(p2) - math.Sin(p1)*math.Cos(p2)*math.Cos(dl)
	return math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360)
}
