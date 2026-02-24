package weather

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"mu/app"
)

const (
	googleWeatherDailyURL  = "https://weather.googleapis.com/v1/forecast/days:lookup"
	googleWeatherHourlyURL = "https://weather.googleapis.com/v1/forecast/hours:lookup"
	googlePollenBaseURL    = "https://pollen.googleapis.com/v1/forecast:lookup"
)

// googleAPIKey returns the Google API key from the environment.
func googleAPIKey() string {
	return os.Getenv("GOOGLE_API_KEY")
}

// httpClient is the shared HTTP client with timeout.
var httpClient = &http.Client{Timeout: 15 * time.Second}

// WeatherForecast holds the parsed forecast data returned by the Google Weather API.
type WeatherForecast struct {
	Location     string
	Current      *CurrentConditions
	HourlyItems  []HourlyItem
	DailyItems   []DailyItem
}

// CurrentConditions holds current weather values.
type CurrentConditions struct {
	TempC       float64
	FeelsLikeC  float64
	Description string
	Humidity    int
	WindKph     float64
	IconCode    string
}

// HourlyItem holds one hour of forecast data.
type HourlyItem struct {
	Time        time.Time
	TempC       float64
	Description string
	IconCode    string
	PrecipMM    float64
}

// DailyItem holds one day of forecast data.
type DailyItem struct {
	Date       time.Time
	MaxTempC   float64
	MinTempC   float64
	Description string
	RainMM     float64
	WillRain   bool
}

// PollenForecast holds pollen data for a location.
type PollenForecast struct {
	Date             time.Time
	GrassIndex       int
	GrassCategory    string
	GrassDescription string
	TreeIndex        int
	TreeCategory     string
	TreeDescription  string
	WeedIndex        int
	WeedCategory     string
	WeedDescription  string
	HealthRecs       []string
}

// --- Google Weather API response structs ---

type googleWeatherResponse struct {
	ForecastDays []googleForecastDay `json:"forecastDays"`
}

type googleForecastDay struct {
	Interval struct {
		StartTime string `json:"startTime"`
	} `json:"interval"`
	DaytimeForecast    *googlePeriodForecast `json:"daytimeForecast"`
	NighttimeForecast  *googlePeriodForecast `json:"nighttimeForecast"`
	MaxTemperature     *googleTemp           `json:"maxTemperature"`
	MinTemperature     *googleTemp           `json:"minTemperature"`
	SunriseTime        string                `json:"sunriseTime"`
	SunsetTime         string                `json:"sunsetTime"`
}

type googlePeriodForecast struct {
	WeatherCondition   *googleWeatherCondition   `json:"weatherCondition"`
	Precipitation      *googlePrecipitation      `json:"precipitation"`
}

type googleWeatherCondition struct {
	Description struct {
		Text string `json:"text"`
	} `json:"description"`
	Type string `json:"type"`
}

type googlePrecipitation struct {
	Probability struct {
		Percent int `json:"percent"`
	} `json:"probability"`
	QpfMillimeters float64 `json:"qpfMillimeters"`
}

type googleTemp struct {
	Degrees float64 `json:"degrees"`
	Unit    string  `json:"unit"`
}

type googleHourlyResponse struct {
	ForecastHours []googleForecastHour `json:"forecastHours"`
}

type googleForecastHour struct {
	Interval struct {
		StartTime string `json:"startTime"`
	} `json:"interval"`
	Temperature      *googleTemp             `json:"temperature"`
	WeatherCondition *googleWeatherCondition `json:"weatherCondition"`
	Precipitation    *googlePrecipitation    `json:"precipitation"`
}

// --- Google Pollen API response structs ---

type googlePollenResponse struct {
	DailyInfo []googlePollenDay `json:"dailyInfo"`
}

type googlePollenDay struct {
	Date struct {
		Year  int `json:"year"`
		Month int `json:"month"`
		Day   int `json:"day"`
	} `json:"date"`
	PollenTypeInfo []googlePollenTypeInfo `json:"pollenTypeInfo"`
}

type googlePollenTypeInfo struct {
	Code        string `json:"code"`
	DisplayName string `json:"displayName"`
	InSeason    bool   `json:"inSeason"`
	IndexInfo   *struct {
		Value            int    `json:"value"`
		DisplayName      string `json:"displayName"`
		Category         string `json:"category"`
		IndexDescription string `json:"indexDescription"`
	} `json:"indexInfo"`
	HealthRecommendations []string `json:"healthRecommendations"`
}

// FetchWeather retrieves weather forecast from the Google Weather API.
// Returns an error when GOOGLE_API_KEY is not set.
func FetchWeather(lat, lon float64) (*WeatherForecast, error) {
	key := googleAPIKey()
	if key == "" {
		return nil, fmt.Errorf("GOOGLE_API_KEY not configured")
	}

	// Fetch daily forecast (10 days)
	dailyURL := fmt.Sprintf("%s?key=%s&location.latitude=%f&location.longitude=%f&days=10&unitsSystem=METRIC",
		googleWeatherDailyURL, key, lat, lon)

	dailyResp, err := googleWeatherGet(dailyURL, "google_weather_daily")
	if err != nil {
		return nil, err
	}

	var dailyData googleWeatherResponse
	if err := json.Unmarshal(dailyResp, &dailyData); err != nil {
		return nil, fmt.Errorf("failed to parse weather response: %w", err)
	}

	// Fetch hourly forecast (24 hours)
	hourlyURL := fmt.Sprintf("%s?key=%s&location.latitude=%f&location.longitude=%f&hours=24&unitsSystem=METRIC",
		googleWeatherHourlyURL, key, lat, lon)

	hourlyResp, err := googleWeatherGet(hourlyURL, "google_weather_hourly")
	if err != nil {
		return nil, err
	}

	var hourlyData googleHourlyResponse
	if err := json.Unmarshal(hourlyResp, &hourlyData); err != nil {
		return nil, fmt.Errorf("failed to parse hourly weather response: %w", err)
	}

	forecast := &WeatherForecast{}

	// Parse daily items
	for _, day := range dailyData.ForecastDays {
		t, err := time.Parse(time.RFC3339, day.Interval.StartTime)
		if err != nil {
			t, _ = time.Parse("2006-01-02T15:04:05Z", day.Interval.StartTime)
		}

		item := DailyItem{Date: t}
		if day.MaxTemperature != nil {
			item.MaxTempC = toCelsius(day.MaxTemperature.Degrees, day.MaxTemperature.Unit)
		}
		if day.MinTemperature != nil {
			item.MinTempC = toCelsius(day.MinTemperature.Degrees, day.MinTemperature.Unit)
		}
		if day.DaytimeForecast != nil {
			if day.DaytimeForecast.WeatherCondition != nil {
				item.Description = day.DaytimeForecast.WeatherCondition.Description.Text
			}
			if day.DaytimeForecast.Precipitation != nil {
				item.RainMM = day.DaytimeForecast.Precipitation.QpfMillimeters
				item.WillRain = day.DaytimeForecast.Precipitation.Probability.Percent >= 30
			}
		}
		forecast.DailyItems = append(forecast.DailyItems, item)
	}

	// Parse hourly items
	for _, h := range hourlyData.ForecastHours {
		t, err := time.Parse(time.RFC3339, h.Interval.StartTime)
		if err != nil {
			t, _ = time.Parse("2006-01-02T15:04:05Z", h.Interval.StartTime)
		}

		item := HourlyItem{Time: t}
		if h.Temperature != nil {
			item.TempC = toCelsius(h.Temperature.Degrees, h.Temperature.Unit)
		}
		if h.WeatherCondition != nil {
			item.Description = h.WeatherCondition.Description.Text
			item.IconCode = h.WeatherCondition.Type
		}
		if h.Precipitation != nil {
			item.PrecipMM = h.Precipitation.QpfMillimeters
		}
		forecast.HourlyItems = append(forecast.HourlyItems, item)
	}

	// Derive current conditions from first hourly item if available
	if len(forecast.HourlyItems) > 0 {
		first := forecast.HourlyItems[0]
		forecast.Current = &CurrentConditions{
			TempC:       first.TempC,
			FeelsLikeC:  first.TempC,
			Description: first.Description,
			IconCode:    first.IconCode,
		}
	} else if len(forecast.DailyItems) > 0 {
		d := forecast.DailyItems[0]
		forecast.Current = &CurrentConditions{
			TempC:       (d.MaxTempC + d.MinTempC) / 2,
			Description: d.Description,
		}
	}

	return forecast, nil
}

// FetchPollen retrieves pollen forecast from the Google Pollen API.
// Returns an error when GOOGLE_API_KEY is not set.
func FetchPollen(lat, lon float64) ([]PollenForecast, error) {
	key := googleAPIKey()
	if key == "" {
		return nil, fmt.Errorf("GOOGLE_API_KEY not configured")
	}

	apiURL := fmt.Sprintf("%s?key=%s&location.latitude=%f&location.longitude=%f&days=5",
		googlePollenBaseURL, key, lat, lon)

	respBody, err := googleWeatherGet(apiURL, "google_pollen")
	if err != nil {
		return nil, err
	}

	var pollenResp googlePollenResponse
	if err := json.Unmarshal(respBody, &pollenResp); err != nil {
		return nil, fmt.Errorf("failed to parse pollen response: %w", err)
	}

	var result []PollenForecast
	for _, day := range pollenResp.DailyInfo {
		pf := PollenForecast{
			Date: time.Date(day.Date.Year, time.Month(day.Date.Month), day.Date.Day, 0, 0, 0, 0, time.UTC),
		}
		for _, pt := range day.PollenTypeInfo {
			idx := 0
			category := "N/A"
			description := ""
			if pt.IndexInfo != nil {
				idx = pt.IndexInfo.Value
				if pt.IndexInfo.Category != "" {
					category = pt.IndexInfo.Category
				} else if pt.IndexInfo.DisplayName != "" {
					category = pt.IndexInfo.DisplayName
				}
				description = pt.IndexInfo.IndexDescription
			}
			switch pt.Code {
			case "GRASS":
				pf.GrassIndex = idx
				pf.GrassCategory = category
				pf.GrassDescription = description
				pf.HealthRecs = append(pf.HealthRecs, pt.HealthRecommendations...)
			case "TREE":
				pf.TreeIndex = idx
				pf.TreeCategory = category
				pf.TreeDescription = description
			case "WEED":
				pf.WeedIndex = idx
				pf.WeedCategory = category
				pf.WeedDescription = description
			}
		}
		result = append(result, pf)
	}

	return result, nil
}

// googleWeatherGet performs a GET request and returns the response body.
func googleWeatherGet(apiURL, service string) ([]byte, error) {
	start := time.Now()
	resp, err := httpClient.Get(apiURL)
	if err != nil {
		app.RecordAPICall(service, "GET", apiURL, 0, time.Since(start), err, "", "")
		return nil, fmt.Errorf("%s request failed: %w", service, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		app.RecordAPICall(service, "GET", apiURL, resp.StatusCode, time.Since(start), err, "", "")
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		callErr := fmt.Errorf("%s returned status %d: %s", service, resp.StatusCode, string(body))
		app.RecordAPICall(service, "GET", apiURL, resp.StatusCode, time.Since(start), callErr, "", string(body))
		return nil, callErr
	}

	app.RecordAPICall(service, "GET", apiURL, resp.StatusCode, time.Since(start), nil, "", string(body))
	return body, nil
}

// toCelsius converts a temperature to Celsius.
func toCelsius(degrees float64, unit string) float64 {
	if unit == "FAHRENHEIT" {
		return (degrees - 32) * 5 / 9
	}
	return degrees
}
