package weather

// Weather without a key, anywhere.
//
// The keyless path used to be the US National Weather Service, which is
// excellent and covers the United States. Everywhere else it returns nothing,
// so an unconfigured instance in London had a weather service that could not
// tell you the weather — and self-hosting means being unconfigured on the first
// run, by definition.
//
// Open-Meteo is global, needs no key, no account and no card, and asks only
// for attribution. That makes it the right default: Mu works out of the box,
// and GOOGLE_API_KEY upgrades it rather than switching it on.
//
// The chain is Google when a key is set, Open-Meteo when it is not, and NWS
// behind Open-Meteo because it was already here and is a good third opinion in
// the country it covers.

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"mu/internal/app"
)

// openMeteoBaseURL is a variable so tests can point it at a stub. A test that
// reaches the real service is slow, flaky and rude to a free provider.
var openMeteoBaseURL = "https://api.open-meteo.com/v1/forecast"

// openMeteoResponse is the subset of the response Mu uses.
type openMeteoResponse struct {
	Timezone string `json:"timezone"`
	Current  struct {
		Time          string  `json:"time"`
		Temperature   float64 `json:"temperature_2m"`
		FeelsLike     float64 `json:"apparent_temperature"`
		Humidity      float64 `json:"relative_humidity_2m"`
		WindSpeed     float64 `json:"wind_speed_10m"`
		WeatherCode   int     `json:"weather_code"`
		Precipitation float64 `json:"precipitation"`
	} `json:"current"`
	Hourly struct {
		Time          []string  `json:"time"`
		Temperature   []float64 `json:"temperature_2m"`
		WeatherCode   []int     `json:"weather_code"`
		Precipitation []float64 `json:"precipitation"`
	} `json:"hourly"`
	Daily struct {
		Time          []string  `json:"time"`
		WeatherCode   []int     `json:"weather_code"`
		TempMax       []float64 `json:"temperature_2m_max"`
		TempMin       []float64 `json:"temperature_2m_min"`
		Precipitation []float64 `json:"precipitation_sum"`
	} `json:"daily"`
}

// wmoDescription turns a WMO weather code into the same kind of phrase the
// other providers return, so the rendering does not care which one answered.
func wmoDescription(code int) string {
	switch code {
	case 0:
		return "Clear"
	case 1:
		return "Mostly clear"
	case 2:
		return "Partly cloudy"
	case 3:
		return "Overcast"
	case 45, 48:
		return "Fog"
	case 51, 53, 55:
		return "Drizzle"
	case 56, 57:
		return "Freezing drizzle"
	case 61, 63, 65:
		return "Rain"
	case 66, 67:
		return "Freezing rain"
	case 71, 73, 75:
		return "Snow"
	case 77:
		return "Snow grains"
	case 80, 81, 82:
		return "Rain showers"
	case 85, 86:
		return "Snow showers"
	case 95:
		return "Thunderstorm"
	case 96, 99:
		return "Thunderstorm with hail"
	}
	return "Unknown"
}

// wmoIcon maps a WMO code onto the icon vocabulary the pages already use.
func wmoIcon(code int) string {
	switch {
	case code == 0 || code == 1:
		return "CLEAR"
	case code == 2 || code == 3:
		return "CLOUDY"
	case code >= 45 && code <= 48:
		return "FOG"
	case code >= 51 && code <= 67:
		return "RAIN"
	case code >= 71 && code <= 77, code >= 85 && code <= 86:
		return "SNOW"
	case code >= 80 && code <= 82:
		return "RAIN"
	case code >= 95:
		return "THUNDERSTORM"
	}
	return "CLOUDY"
}

// fetchOpenMeteoWeather returns a forecast for anywhere, without a key.
func fetchOpenMeteoWeather(lat, lon float64) (*WeatherForecast, error) {
	url := fmt.Sprintf("%s?latitude=%f&longitude=%f"+
		"&current=temperature_2m,apparent_temperature,relative_humidity_2m,wind_speed_10m,weather_code,precipitation"+
		"&hourly=temperature_2m,weather_code,precipitation"+
		"&daily=weather_code,temperature_2m_max,temperature_2m_min,precipitation_sum"+
		"&forecast_days=10&timezone=auto", openMeteoBaseURL, lat, lon)

	body, err := weatherGet(url, "open_meteo")
	if err != nil {
		app.Log("weather", "open-meteo failed (%v), trying NWS", err)
		return fetchNWSWeather(lat, lon)
	}

	var r openMeteoResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("failed to parse open-meteo response: %w", err)
	}

	f := &WeatherForecast{
		Location:    fmt.Sprintf("%.4f, %.4f", lat, lon),
		Source:      "Open-Meteo",
		GeneratedAt: time.Now(),
	}

	// Open-Meteo returns local times without a zone; they are in the zone it
	// resolved from the coordinates, which is what a person reading a forecast
	// wants to see.
	parse := func(s string) time.Time {
		for _, layout := range []string{"2006-01-02T15:04", "2006-01-02"} {
			if t, err := time.Parse(layout, s); err == nil {
				return t
			}
		}
		return time.Time{}
	}

	f.Current = &CurrentConditions{
		TempC:             math.Round(r.Current.Temperature*10) / 10,
		FeelsLikeC:        math.Round(r.Current.FeelsLike*10) / 10,
		Description:       wmoDescription(r.Current.WeatherCode),
		Humidity:          int(math.Round(r.Current.Humidity)),
		HumidityAvailable: true,
		WindKph:           math.Round(r.Current.WindSpeed*10) / 10,
		WindKphAvailable:  true,
		IconCode:          wmoIcon(r.Current.WeatherCode),
	}
	f.ObservedAt = parse(r.Current.Time)

	now := time.Now()
	for i := range r.Hourly.Time {
		when := parse(r.Hourly.Time[i])
		// Only what is still ahead, and only the next day of it.
		if when.Before(now.Add(-time.Hour)) || when.After(now.Add(24*time.Hour)) {
			continue
		}
		item := HourlyItem{Time: when}
		if i < len(r.Hourly.Temperature) {
			item.TempC = math.Round(r.Hourly.Temperature[i]*10) / 10
		}
		if i < len(r.Hourly.WeatherCode) {
			item.Description = wmoDescription(r.Hourly.WeatherCode[i])
			item.IconCode = wmoIcon(r.Hourly.WeatherCode[i])
		}
		if i < len(r.Hourly.Precipitation) {
			item.PrecipMM = r.Hourly.Precipitation[i]
		}
		f.HourlyItems = append(f.HourlyItems, item)
	}

	for i := range r.Daily.Time {
		item := DailyItem{Date: parse(r.Daily.Time[i])}
		if i < len(r.Daily.TempMax) {
			item.MaxTempC = math.Round(r.Daily.TempMax[i]*10) / 10
		}
		if i < len(r.Daily.TempMin) {
			item.MinTempC = math.Round(r.Daily.TempMin[i]*10) / 10
		}
		if i < len(r.Daily.WeatherCode) {
			item.Description = wmoDescription(r.Daily.WeatherCode[i])
		}
		if i < len(r.Daily.Precipitation) {
			item.RainMM = r.Daily.Precipitation[i]
			item.WillRain = r.Daily.Precipitation[i] > 0.2
		}
		f.DailyItems = append(f.DailyItems, item)
	}

	// A response that parsed but carried nothing is a failure, not an empty
	// forecast. Current is always populated by the assignment above, so it
	// cannot be the test — something answering with the wrong JSON shape would
	// have produced a forecast of zeroes and looked like a cold, clear day.
	if len(f.DailyItems) == 0 && len(f.HourlyItems) == 0 {
		app.Log("weather", "open-meteo returned nothing usable, trying NWS")
		return fetchNWSWeather(lat, lon)
	}
	return f, nil
}
