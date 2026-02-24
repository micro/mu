package weather

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"mu/app"
	"mu/auth"
	"mu/wallet"
)

// Load initialises the weather package (placeholder for future caching).
func Load() {}

// Handler handles /weather requests.
func Handler(w http.ResponseWriter, r *http.Request) {
	if app.WantsJSON(r) {
		handleJSON(w, r)
		return
	}
	handleHTML(w, r)
}

// handleJSON handles JSON API requests for weather data.
func handleJSON(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")
	if latStr == "" || lonStr == "" {
		app.RespondError(w, http.StatusBadRequest, "lat and lon are required")
		return
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		app.RespondError(w, http.StatusBadRequest, "invalid lat")
		return
	}
	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		app.RespondError(w, http.StatusBadRequest, "invalid lon")
		return
	}

	includePollen := r.URL.Query().Get("pollen") == "1"

	// Check quota for weather forecast
	canProceed, useFree, cost, _ := wallet.CheckQuota(acc.ID, wallet.OpWeatherForecast)
	if !canProceed {
		app.RespondError(w, http.StatusPaymentRequired, "Insufficient credits. Top up your wallet to continue.")
		return
	}

	// Fetch weather
	forecast, err := FetchWeather(lat, lon)
	if err != nil {
		app.RespondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to fetch weather: %v", err))
		return
	}

	// Consume weather quota
	if useFree {
		wallet.UseFreeSearch(acc.ID)
	} else if cost > 0 {
		wallet.DeductCredits(acc.ID, cost, wallet.OpWeatherForecast, nil)
	}

	result := map[string]interface{}{
		"forecast": forecast,
	}

	// Fetch pollen if requested and quota allows
	if includePollen {
		canPollenProceed, usePollenFree, pollenCost, _ := wallet.CheckQuota(acc.ID, wallet.OpWeatherPollen)
		if canPollenProceed {
			pollen, pollenErr := FetchPollen(lat, lon)
			if pollenErr == nil {
				result["pollen"] = pollen
				if usePollenFree {
					wallet.UseFreeSearch(acc.ID)
				} else if pollenCost > 0 {
					wallet.DeductCredits(acc.ID, pollenCost, wallet.OpWeatherPollen, nil)
				}
			}
		}
	}

	app.RespondJSON(w, result)
}

// handleHTML renders the weather page.
func handleHTML(w http.ResponseWriter, r *http.Request) {
	body := renderWeatherPage(r)
	app.Respond(w, r, app.Response{
		Title:       "Weather",
		Description: "Local weather forecast with hourly and 10-day outlook",
		HTML:        body,
	})
}

// renderWeatherPage generates the weather page HTML.
func renderWeatherPage(r *http.Request) string {
	sess, err := auth.GetSession(r)
	isAuthed := err == nil && sess != nil

	var sb strings.Builder

	if !isAuthed {
		sb.WriteString(`<p>Please <a href="/login">log in</a> to use Weather.</p>`)
		return sb.String()
	}

	// Cost info
	sb.WriteString(`<p class="card-desc">Get the local weather forecast for your area. `)
	sb.WriteString(fmt.Sprintf(`Costs %dp per lookup`, wallet.CostWeatherForecast))
	sb.WriteString(`; enable pollen data for `)
	sb.WriteString(fmt.Sprintf(`+%dp more.</p>`, wallet.CostWeatherPollen))

	// Weather page with location search
	sb.WriteString(`
<div id="weather-app">
  <div class="weather-controls">
    <button id="btn-locate" onclick="weatherLocate()" class="btn">Use My Location</button>
    <span class="weather-or">or</span>
    <form id="form-search" onsubmit="weatherSearch(event)" class="weather-search-form">
      <input id="input-location" type="text" placeholder="Search city or postcode" class="weather-search-input">
      <button type="submit" class="btn">Search</button>
    </form>
  </div>

  <div class="weather-options" style="margin-top:12px;">
    <label style="display:flex;align-items:center;gap:6px;cursor:pointer;">
      <input type="checkbox" id="toggle-pollen" onchange="weatherTogglePollen()">
      <span>Include pollen forecast (+` + fmt.Sprintf("%dp", wallet.CostWeatherPollen) + `)</span>
    </label>
  </div>

  <div id="weather-loading" style="display:none;" class="card-meta">Loading weather…</div>
  <div id="weather-error" style="display:none;" class="text-error"></div>
  <div id="weather-result" style="display:none;">
    <div id="weather-current"></div>
    <div id="weather-hourly"></div>
    <div id="weather-daily"></div>
    <div id="weather-pollen"></div>
  </div>
</div>

<script>
(function() {
  var pollenEnabled = false;

  function weatherTogglePollen() {
    pollenEnabled = document.getElementById('toggle-pollen').checked;
  }

  function weatherLocate() {
    if (!navigator.geolocation) {
      showError('Geolocation is not supported by your browser.');
      return;
    }
    showLoading(true);
    navigator.geolocation.getCurrentPosition(function(pos) {
      fetchWeather(pos.coords.latitude, pos.coords.longitude);
    }, function(err) {
      showLoading(false);
      showError('Location access denied. Please search by city name instead.');
    });
  }

  function weatherSearch(e) {
    e.preventDefault();
    var q = document.getElementById('input-location').value.trim();
    if (!q) return;
    showLoading(true);
    // Geocode via nominatim
    fetch('https://nominatim.openstreetmap.org/search?q=' + encodeURIComponent(q) + '&format=json&limit=1', {
      headers: {'Accept': 'application/json', 'User-Agent': 'MuWeatherApp/1.0'}
    }).then(function(r){ return r.json(); }).then(function(data) {
      if (!data || data.length === 0) {
        showLoading(false);
        showError('Location not found. Please try a different search.');
        return;
      }
      fetchWeather(parseFloat(data[0].lat), parseFloat(data[0].lon));
    }).catch(function() {
      showLoading(false);
      showError('Failed to find location.');
    });
  }

  function fetchWeather(lat, lon) {
    showLoading(true);
    showError('');
    var url = '/weather?lat=' + lat + '&lon=' + lon + (pollenEnabled ? '&pollen=1' : '');
    fetch(url, {headers: {'Accept': 'application/json'}})
      .then(function(r) {
        if (!r.ok) return r.json().then(function(e){ throw e; });
        return r.json();
      })
      .then(function(data) {
        showLoading(false);
        renderWeather(data);
      })
      .catch(function(err) {
        showLoading(false);
        var msg = (err && err.error) ? err.error : 'Failed to load weather data.';
        if (msg.indexOf('Insufficient credits') !== -1) {
          msg += ' <a href="/wallet/topup">Top up your wallet</a>.';
        }
        showError(msg);
      });
  }

  function renderWeather(data) {
    var f = data.forecast;
    document.getElementById('weather-result').style.display = '';

    // Current conditions
    var cur = '';
    if (f && f.Current) {
      var c = f.Current;
      cur += '<div class="card weather-current">';
      cur += '<div class="weather-temp">' + Math.round(c.TempC) + '°C</div>';
      cur += '<div class="weather-desc">' + (c.Description || '') + '</div>';
      if (c.Humidity) cur += '<div class="card-meta">Humidity: ' + c.Humidity + '%</div>';
      if (c.WindKph) cur += '<div class="card-meta">Wind: ' + c.WindKph.toFixed(1) + ' km/h</div>';
      cur += '</div>';
    }
    document.getElementById('weather-current').innerHTML = cur;

    // Hourly forecast
    var hourly = '';
    if (f && f.HourlyItems && f.HourlyItems.length > 0) {
      hourly += '<h3>Hourly Forecast</h3>';
      hourly += '<div class="weather-hourly-row">';
      var items = f.HourlyItems.slice(0, 24);
      items.forEach(function(h) {
        var t = new Date(h.Time);
        var timeStr = t.toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'});
        hourly += '<div class="weather-hour-card">';
        hourly += '<div class="weather-hour-time">' + timeStr + '</div>';
        hourly += '<div class="weather-hour-temp">' + Math.round(h.TempC) + '°C</div>';
        hourly += '<div class="weather-hour-desc">' + (h.Description || '') + '</div>';
        if (h.PrecipMM > 0) hourly += '<div class="weather-hour-rain">🌧 ' + h.PrecipMM.toFixed(1) + 'mm</div>';
        hourly += '</div>';
      });
      hourly += '</div>';
    }
    document.getElementById('weather-hourly').innerHTML = hourly;

    // Daily forecast table
    var daily = '';
    if (f && f.DailyItems && f.DailyItems.length > 0) {
      daily += '<h3>10-Day Forecast</h3>';
      daily += '<div class="table-scroll"><table class="data-table weather-table">';
      daily += '<thead><tr><th>Date</th><th>Conditions</th><th>High</th><th>Low</th><th>Rain</th></tr></thead>';
      daily += '<tbody>';
      f.DailyItems.forEach(function(d) {
        var dt = new Date(d.Date);
        var dateStr = dt.toLocaleDateString([], {weekday: 'short', month: 'short', day: 'numeric'});
        var rain = d.WillRain ? ('🌧 ' + (d.RainMM > 0 ? d.RainMM.toFixed(1) + 'mm' : 'Yes')) : '—';
        daily += '<tr>';
        daily += '<td>' + dateStr + '</td>';
        daily += '<td>' + (d.Description || '—') + '</td>';
        daily += '<td>' + Math.round(d.MaxTempC) + '°C</td>';
        daily += '<td>' + Math.round(d.MinTempC) + '°C</td>';
        daily += '<td>' + rain + '</td>';
        daily += '</tr>';
      });
      daily += '</tbody></table></div>';
    }
    document.getElementById('weather-daily').innerHTML = daily;

    // Pollen data
    var pollen = '';
    if (data.pollen && data.pollen.length > 0) {
      pollen += '<h3>Pollen Forecast</h3>';
      pollen += '<div class="table-scroll"><table class="data-table weather-table">';
      pollen += '<thead><tr><th>Date</th><th>Grass</th><th>Tree</th><th>Weed</th></tr></thead>';
      pollen += '<tbody>';
      data.pollen.forEach(function(p) {
        var dt = new Date(p.Date);
        var dateStr = dt.toLocaleDateString([], {weekday: 'short', month: 'short', day: 'numeric'});
        pollen += '<tr>';
        pollen += '<td>' + dateStr + '</td>';
        pollen += '<td>' + pollenBadge(p.GrassIndex, p.GrassLabel) + '</td>';
        pollen += '<td>' + pollenBadge(p.TreeIndex, p.TreeLabel) + '</td>';
        pollen += '<td>' + pollenBadge(p.WeedIndex, p.WeedLabel) + '</td>';
        pollen += '</tr>';
      });
      pollen += '</tbody></table></div>';
    }
    document.getElementById('weather-pollen').innerHTML = pollen;
  }

  function pollenBadge(index, label) {
    if (!label || label === 'N/A') return '—';
    var colors = ['', '#aed6f1','#82e0aa','#f9e79f','#f0b27a','#ec7063'];
    var color = colors[Math.min(index, colors.length-1)] || '#ddd';
    return '<span style="background:' + color + ';padding:2px 6px;border-radius:4px;font-size:0.85em;">' + label + '</span>';
  }

  function showLoading(on) {
    document.getElementById('weather-loading').style.display = on ? '' : 'none';
    if (on) document.getElementById('weather-result').style.display = 'none';
  }

  function showError(msg) {
    var el = document.getElementById('weather-error');
    el.innerHTML = msg;
    el.style.display = msg ? '' : 'none';
  }

  // Expose to global scope for onclick handlers
  window.weatherLocate = weatherLocate;
  window.weatherSearch = weatherSearch;
  window.weatherTogglePollen = weatherTogglePollen;
})();
</script>
`)

	return sb.String()
}
