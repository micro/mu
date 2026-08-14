package weather

// The half of the atmosphere Google does not sell us.
//
// Forecast goes through Google Weather, which is keyed and metered, and a
// self-hosted instance without a key gets a US-only fallback. Air, Marine and
// History go through the Open-Meteo family instead — same institution behind
// the forecast one this service already falls back to, no key, and covering
// three questions Google's weather product does not answer at all.
//
// That makes them free, in the sense quota.json means it: a call is charged
// when it costs us something, and these cost nothing. It also makes them the
// part of weather that still works on a clone.
//
// Three separate hosts rather than one, because Open-Meteo splits by model —
// air quality comes from CAMS, waves from a wave model, history from ERA5
// reanalysis. They are one family with one response shape, which is why they
// share a fetcher here.

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// Variables rather than constants so a test can point them at a stub, which is
// how the rest of this package is tested.
var (
	airURL     = "https://air-quality-api.open-meteo.com/v1/air-quality"
	marineURL  = "https://marine-api.open-meteo.com/v1/marine"
	archiveURL = "https://archive-api.open-meteo.com/v1/archive"
)

// openMeteoClient is patient in a way the forecast client does not need to be.
//
// The archive answers from reanalysis rather than a cached forecast, so a year
// of daily values is real work at the other end, and the handshake to that host
// alone has been measured past the ten seconds Go's default transport allows.
// The forecast path keeps its tighter budget: a page is waiting on it.
var openMeteoClient = &http.Client{
	Timeout: 45 * time.Second,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 40 * time.Second,
	},
}

// openMeteoGet fetches and decodes, without recording spend.
//
// Deliberately not weatherGet: that one ends every call with
// RecordUsage("google", …), so routing a free Open-Meteo request through it
// would bill this instance's own reporting for a request nobody was invoiced
// for.
func openMeteoGet(url string, into any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "mu/1.0 (+https://github.com/micro/mu)")

	resp, err := openMeteoClient.Do(req)
	if err != nil {
		return fmt.Errorf("weather data is unreachable: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		// Open-Meteo puts the actual complaint in the body — a bad date range
		// or an unknown field — and it is more use to a caller than the code.
		var e struct {
			Reason string `json:"reason"`
		}
		if json.Unmarshal(body, &e) == nil && e.Reason != "" {
			return fmt.Errorf("%s", e.Reason)
		}
		return fmt.Errorf("weather data returned %d", resp.StatusCode)
	}
	return json.Unmarshal(body, into)
}

// AirQuality is what is in the air at a place right now.
type AirQuality struct {
	PM25, PM10       float64
	Ozone, NO2, SO2  float64
	UV               float64
	EuropeanAQI      int
	USAQI            int
	Pollen           map[string]float64
	When             time.Time
	Timezone         string
	HavePollen       bool
	HaveEuropeanAQI  bool
	HaveUSAQIReading bool
}

// airQuality fetches current air quality and pollen.
func airQuality(lat, lon float64) (*AirQuality, error) {
	url := fmt.Sprintf("%s?latitude=%f&longitude=%f&timezone=auto&current="+
		"pm2_5,pm10,ozone,nitrogen_dioxide,sulphur_dioxide,uv_index,european_aqi,us_aqi,"+
		"alder_pollen,birch_pollen,grass_pollen,mugwort_pollen,olive_pollen,ragweed_pollen",
		airURL, lat, lon)

	var r struct {
		Timezone string `json:"timezone"`
		Current  struct {
			Time    string   `json:"time"`
			PM25    float64  `json:"pm2_5"`
			PM10    float64  `json:"pm10"`
			Ozone   float64  `json:"ozone"`
			NO2     float64  `json:"nitrogen_dioxide"`
			SO2     float64  `json:"sulphur_dioxide"`
			UV      float64  `json:"uv_index"`
			EAQI    *float64 `json:"european_aqi"`
			USAQI   *float64 `json:"us_aqi"`
			Alder   *float64 `json:"alder_pollen"`
			Birch   *float64 `json:"birch_pollen"`
			Grass   *float64 `json:"grass_pollen"`
			Mugwort *float64 `json:"mugwort_pollen"`
			Olive   *float64 `json:"olive_pollen"`
			Ragweed *float64 `json:"ragweed_pollen"`
		} `json:"current"`
	}
	if err := openMeteoGet(url, &r); err != nil {
		return nil, err
	}

	a := &AirQuality{
		PM25: r.Current.PM25, PM10: r.Current.PM10,
		Ozone: r.Current.Ozone, NO2: r.Current.NO2, SO2: r.Current.SO2,
		UV: r.Current.UV, Timezone: r.Timezone,
		Pollen: map[string]float64{},
	}
	if t, err := time.Parse("2006-01-02T15:04", r.Current.Time); err == nil {
		a.When = t
	}
	if v := r.Current.EAQI; v != nil {
		a.EuropeanAQI, a.HaveEuropeanAQI = int(math.Round(*v)), true
	}
	if v := r.Current.USAQI; v != nil {
		a.USAQI, a.HaveUSAQIReading = int(math.Round(*v)), true
	}

	// Pollen is a European model. Outside its area the fields come back null
	// rather than zero, and reporting "no pollen" where nobody counted it
	// would be a lie a hay fever sufferer could act on.
	for name, v := range map[string]*float64{
		"alder": r.Current.Alder, "birch": r.Current.Birch,
		"grass": r.Current.Grass, "mugwort": r.Current.Mugwort,
		"olive": r.Current.Olive, "ragweed": r.Current.Ragweed,
	} {
		if v != nil {
			a.Pollen[name] = *v
			a.HavePollen = true
		}
	}
	return a, nil
}

// aqiWord turns a European AQI number into the word the scale publishes.
func aqiWord(aqi int) string {
	switch {
	case aqi <= 20:
		return "good"
	case aqi <= 40:
		return "fair"
	case aqi <= 60:
		return "moderate"
	case aqi <= 80:
		return "poor"
	case aqi <= 100:
		return "very poor"
	}
	return "extremely poor"
}

// pollenWord bands a grain count the way a pollen forecast does.
func pollenWord(grains float64) string {
	switch {
	case grains < 1:
		return "none"
	case grains < 20:
		return "low"
	case grains < 50:
		return "moderate"
	case grains < 150:
		return "high"
	}
	return "very high"
}

// Sea is the state of the water at a coastal point.
type Sea struct {
	WaveHeight, WavePeriod float64
	WaveDirection          float64
	SwellHeight            float64
	SwellPeriod            float64
	SeaTempC               float64
	HaveSeaTemp            bool
	When                   time.Time
	Days                   []SeaDay
}

// SeaDay is one day of the wave forecast.
type SeaDay struct {
	Date         time.Time
	MaxHeight    float64
	MaxPeriod    float64
	DominantFrom float64
}

// sea fetches current wave conditions and the days ahead.
func sea(lat, lon float64, days int) (*Sea, error) {
	if days <= 0 || days > 7 {
		days = 3
	}
	url := fmt.Sprintf("%s?latitude=%f&longitude=%f&timezone=auto&forecast_days=%d"+
		"&current=wave_height,wave_period,wave_direction,swell_wave_height,swell_wave_period,sea_surface_temperature"+
		"&daily=wave_height_max,wave_period_max,wave_direction_dominant",
		marineURL, lat, lon, days)

	var r struct {
		Current struct {
			Time        string   `json:"time"`
			Height      float64  `json:"wave_height"`
			Period      float64  `json:"wave_period"`
			Direction   float64  `json:"wave_direction"`
			SwellHeight float64  `json:"swell_wave_height"`
			SwellPeriod float64  `json:"swell_wave_period"`
			SeaTemp     *float64 `json:"sea_surface_temperature"`
		} `json:"current"`
		Daily struct {
			Time      []string  `json:"time"`
			MaxHeight []float64 `json:"wave_height_max"`
			MaxPeriod []float64 `json:"wave_period_max"`
			Dominant  []float64 `json:"wave_direction_dominant"`
		} `json:"daily"`
	}
	if err := openMeteoGet(url, &r); err != nil {
		return nil, err
	}

	// A point inland is in the model's grid but has no water in it, so every
	// field comes back zero. That is not a flat calm, and saying so would be
	// worse than saying nothing.
	if r.Current.Height == 0 && r.Current.Period == 0 && len(r.Daily.Time) > 0 &&
		allZero(r.Daily.MaxHeight) {
		return nil, fmt.Errorf("no sea state here — this looks like an inland point")
	}

	s := &Sea{
		WaveHeight: r.Current.Height, WavePeriod: r.Current.Period,
		WaveDirection: r.Current.Direction,
		SwellHeight:   r.Current.SwellHeight, SwellPeriod: r.Current.SwellPeriod,
	}
	if v := r.Current.SeaTemp; v != nil {
		s.SeaTempC, s.HaveSeaTemp = *v, true
	}
	if t, err := time.Parse("2006-01-02T15:04", r.Current.Time); err == nil {
		s.When = t
	}
	for i, d := range r.Daily.Time {
		day := SeaDay{}
		if t, err := time.Parse("2006-01-02", d); err == nil {
			day.Date = t
		}
		if i < len(r.Daily.MaxHeight) {
			day.MaxHeight = r.Daily.MaxHeight[i]
		}
		if i < len(r.Daily.MaxPeriod) {
			day.MaxPeriod = r.Daily.MaxPeriod[i]
		}
		if i < len(r.Daily.Dominant) {
			day.DominantFrom = r.Daily.Dominant[i]
		}
		s.Days = append(s.Days, day)
	}
	return s, nil
}

func allZero(v []float64) bool {
	for _, f := range v {
		if f != 0 {
			return false
		}
	}
	return true
}

// compass renders degrees as the direction somebody would say.
func compass(deg float64) string {
	points := []string{"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE",
		"S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW"}
	i := int(math.Mod(deg/22.5+0.5, 16))
	if i < 0 {
		i += 16
	}
	return points[i]
}

// PastDay is what the weather actually was on one day.
type PastDay struct {
	Date       time.Time
	MaxC, MinC float64
	RainMM     float64
	WindKph    float64
}

// PastPeriod is a stretch of days and what they averaged.
type PastPeriod struct {
	Days      []PastDay
	MeanMaxC  float64
	MeanMinC  float64
	TotalRain float64
	WettestMM float64
	Wettest   time.Time
	HottestC  float64
	Hottest   time.Time
}

// past fetches recorded weather for a date range.
//
// This is the climate half rather than the forecast half: ERA5 reanalysis,
// which is the record of what happened rather than a guess at what will. It
// runs about five days behind the present, which is why a range ending today
// comes back short rather than empty.
func past(lat, lon float64, from, to string) (*PastPeriod, error) {
	start, err := time.Parse("2006-01-02", strings.TrimSpace(from))
	if err != nil {
		return nil, fmt.Errorf("start must be a date like 2025-08-01")
	}
	end, err := time.Parse("2006-01-02", strings.TrimSpace(to))
	if err != nil {
		return nil, fmt.Errorf("end must be a date like 2025-08-31")
	}
	if end.Before(start) {
		return nil, fmt.Errorf("the end date is before the start date")
	}
	if end.Sub(start) > 366*24*time.Hour {
		return nil, fmt.Errorf("ask for a year or less at a time")
	}

	url := fmt.Sprintf("%s?latitude=%f&longitude=%f&start_date=%s&end_date=%s&timezone=auto"+
		"&daily=temperature_2m_max,temperature_2m_min,precipitation_sum,wind_speed_10m_max",
		archiveURL, lat, lon, start.Format("2006-01-02"), end.Format("2006-01-02"))

	var r struct {
		Daily struct {
			Time []string  `json:"time"`
			Max  []float64 `json:"temperature_2m_max"`
			Min  []float64 `json:"temperature_2m_min"`
			Rain []float64 `json:"precipitation_sum"`
			Wind []float64 `json:"wind_speed_10m_max"`
		} `json:"daily"`
	}
	if err := openMeteoGet(url, &r); err != nil {
		return nil, err
	}

	p := &PastPeriod{}
	var sumMax, sumMin float64
	var counted int
	for i, d := range r.Daily.Time {
		day := PastDay{}
		if t, err := time.Parse("2006-01-02", d); err == nil {
			day.Date = t
		}
		if i < len(r.Daily.Max) {
			day.MaxC = r.Daily.Max[i]
		}
		if i < len(r.Daily.Min) {
			day.MinC = r.Daily.Min[i]
		}
		if i < len(r.Daily.Rain) {
			day.RainMM = r.Daily.Rain[i]
		}
		if i < len(r.Daily.Wind) {
			day.WindKph = r.Daily.Wind[i]
		}
		p.Days = append(p.Days, day)

		sumMax += day.MaxC
		sumMin += day.MinC
		p.TotalRain += day.RainMM
		counted++
		if day.RainMM > p.WettestMM {
			p.WettestMM, p.Wettest = day.RainMM, day.Date
		}
		if counted == 1 || day.MaxC > p.HottestC {
			p.HottestC, p.Hottest = day.MaxC, day.Date
		}
	}
	if counted == 0 {
		return nil, fmt.Errorf("no records for those dates — the archive runs a few days behind today")
	}
	p.MeanMaxC = math.Round(sumMax/float64(counted)*10) / 10
	p.MeanMinC = math.Round(sumMin/float64(counted)*10) / 10
	p.TotalRain = math.Round(p.TotalRain*10) / 10
	return p, nil
}
