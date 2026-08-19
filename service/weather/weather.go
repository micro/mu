package weather

import (
	"net/http"
	"strconv"

	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/quota"
	"mu/internal/service"
)

// Load initialises the weather package and registers its go-micro service.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("weather", "service register failed: %v", err)
	}
}

// CardHTML returns the weather card for the home screen.
// Only fetches for logged-in users. Caches in localStorage for 30 min.
// First visit: user clicks to enable location. After that, auto-refreshes.
func CardHTML() string {
	return `<div id="weather-card">
<div id="weather-card-content" style="font-size:13px;color:#888">
<span id="weather-card-loading"></span>
</div>
<script>
(function(){
var el=document.getElementById('weather-card-content');
var load=document.getElementById('weather-card-loading');
var KEY='mu_weather',KEY_TS='mu_weather_ts',KEY_LAT='mu_weather_lat',KEY_LON='mu_weather_lon',TTL=3600000;
function isLoggedIn(){return document.cookie.indexOf('csrf_token=')!==-1}
if(!isLoggedIn()){load.innerHTML='<a href="/login" style="color:#888">Log in</a> for weather';return}
var cached=localStorage.getItem(KEY);
var ts=parseInt(localStorage.getItem(KEY_TS)||'0');
var stale=!cached||(Date.now()-ts)>=TTL;
if(cached){el.innerHTML=cached}
if(!stale){return}
var savedLat=localStorage.getItem(KEY_LAT);
var savedLon=localStorage.getItem(KEY_LON);
if(savedLat&&savedLon){fetchWeather(savedLat,savedLon);return}
if(!navigator.geolocation){if(!cached){load.textContent='Location not available'};return}
if(!cached){
load.innerHTML='<a href="#" onclick="muWeatherEnable();return false" style="color:#555">Enable location for weather</a>';
window.muWeatherEnable=function(){load.textContent='Checking weather...';getLocation()};
return}
getLocation();
function getLocation(){
navigator.geolocation.getCurrentPosition(function(pos){
var lat=pos.coords.latitude.toFixed(4);
var lon=pos.coords.longitude.toFixed(4);
localStorage.setItem(KEY_LAT,lat);
localStorage.setItem(KEY_LON,lon);
fetchWeather(lat,lon);
},function(){},{timeout:5000});
}
function renderWeather(h){
h='<div style="position:relative">'+h+'<a href="#" onclick="muWeatherRefresh();return false" style="position:absolute;top:0;right:0;font-size:11px;color:#aaa">Refresh</a></div>';
el.innerHTML=h;
localStorage.setItem(KEY,h);
localStorage.setItem(KEY_TS,String(Date.now()));
}
window.muWeatherRefresh=function(){
localStorage.removeItem(KEY);localStorage.removeItem(KEY_TS);localStorage.removeItem(KEY_LAT);localStorage.removeItem(KEY_LON);
el.innerHTML='<span style="color:#888">Refreshing weather...</span>';
if(navigator.geolocation){navigator.geolocation.getCurrentPosition(function(pos){
var lat=pos.coords.latitude.toFixed(4);var lon=pos.coords.longitude.toFixed(4);
localStorage.setItem(KEY_LAT,lat);localStorage.setItem(KEY_LON,lon);
fetchWeather(lat,lon);
},function(){el.innerHTML='<span style="color:#888">Location not available</span>'},{timeout:5000})}
};
function fetchWeather(lat,lon){
fetch('/weather?lat='+lat+'&lon='+lon,{headers:{'Accept':'application/json'}})
.then(function(r){if(!r.ok)throw new Error(r.status);return r.json()})
.then(function(d){
var f=d.forecast;
if(!f||!f.Current){return}
var c=f.Current;
try{localStorage.setItem('mu_weather_now',JSON.stringify({temp:Math.round(c.TempC),desc:c.Description||''}))}catch(e){}
var h='<div style="display:flex;align-items:center;gap:8px">';
h+='<span style="font-size:22px;font-weight:600;color:#333">'+Math.round(c.TempC)+'°C</span>';
h+='<span style="color:#666">'+c.Description+'</span>';
h+='</div>';
if(f.Location)h+='<div style="font-size:12px;color:#999;margin-top:2px">'+f.Location+'</div>';
if(f.DailyItems&&f.DailyItems.length>0){
h+='<div style="display:flex;gap:12px;margin-top:6px;font-size:12px;color:#888">';
for(var i=0;i<Math.min(3,f.DailyItems.length);i++){
var day=f.DailyItems[i];
var name=new Date(day.Date).toLocaleDateString('en',{weekday:'short'});
h+='<span>'+name+' '+Math.round(day.MaxTempC)+'°/'+Math.round(day.MinTempC)+'°</span>';
}
h+='</div>';
}
renderWeather(h);
}).catch(function(){if(!cached){el.innerHTML='<span style="color:#888">Weather unavailable</span>';}});
}
})();
</script>
</div>`
}

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
	if !validCoordinates(lat, lon) {
		app.RespondError(w, http.StatusBadRequest, "lat must be between -90 and 90 and lon must be between -180 and 180")
		return
	}

	includePollen := r.URL.Query().Get("pollen") == "1"

	// Who pays, if anybody does. A forecast is free on an instance that cannot
	// charge, so a guest gets one rather than a sign-in page.
	caller, ok := app.BillableCaller(w, r, quota.OpWeatherForecast)
	if !ok {
		return
	}

	// Fetch weather
	forecast, err := FetchWeather(lat, lon)
	if err != nil {
		app.RespondError(w, http.StatusServiceUnavailable, weatherUnavailableMessage)
		return
	}

	app.Charge(caller, quota.OpWeatherForecast)

	result := map[string]interface{}{
		"forecast": forecast,
	}

	// Air quality comes free with the same coordinates, so it is not a toggle
	// and not a second charge — a toggle only earns its place when saying yes
	// costs the reader something. If the model is down the forecast still
	// stands, which is why this failure is silent.
	if air, err := airQuality(lat, lon); err == nil {
		a := map[string]interface{}{
			"pm25": air.PM25, "pm10": air.PM10,
			"ozone": air.Ozone, "no2": air.NO2, "uv": air.UV,
		}
		if air.HaveEuropeanAQI {
			a["aqi"] = air.EuropeanAQI
			a["aqi_scale"] = "European AQI"
			a["aqi_word"] = aqiWord(air.EuropeanAQI)
		} else if air.HaveUSAQIReading {
			a["aqi"] = air.USAQI
			a["aqi_scale"] = "US AQI"
		}
		result["air"] = a
	}

	// Pollen is a second charge and a second question, asked only if the first
	// one was answered. A caller who cannot afford it still keeps the forecast.
	if includePollen {
		affordable := caller == "" || !quota.Metered(quota.OpWeatherPollen)
		if !affordable {
			affordable, _, _, _ = quota.CheckQuota(caller, quota.OpWeatherPollen)
		}
		if affordable {
			if pollen, err := FetchPollen(lat, lon); err == nil {
				result["pollen"] = pollen
				app.Charge(caller, quota.OpWeatherPollen)
			}
		}
	}

	app.RespondJSON(w, result)
}

// handleHTML renders the weather page.
// handleHTML is the page, derived from the Spec.
//
// It was 297 lines of hand-written HTML — a location box, a forecast table, an
// hourly strip, a pollen panel, a guest variant — and none of it said anything
// the card does not, in a layout only this page used. See api.ServicePage for
// the argument; the short version is that a service you look at and leave
// should be shown by its card, and one you do something in is an app.
func handleHTML(w http.ResponseWriter, r *http.Request) {
	api.ServicePage(w, r, Spec)
}
