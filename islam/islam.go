package islam

import (
	"encoding/json"
	"fmt"
	"html"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/data"
	"mu/internal/event"
	"mu/internal/service"
)

var (
	reminderMutex sync.RWMutex
	reminderHTML  string
)

// Load initializes the reminder data
func Load() {
	if err := service.Register("islam", new(Server)); err != nil {
		app.Log("reminder", "service register failed: %v", err)
	}

	// Load cached HTML
	b, err := data.LoadFile("reminder.html")
	if err == nil {
		reminderMutex.Lock()
		reminderHTML = string(b)
		reminderMutex.Unlock()
	}

	// Start background refresh
	go refreshReminder()
}

func refreshReminder() {
	for {
		fetchReminder()
		time.Sleep(time.Hour)
	}
}

func fetchReminder() {
	app.Log("reminder", "Fetching reminder")

	resp, err := http.Get("https://reminder.dev/api/latest")
	if err != nil {
		app.Log("reminder", "Error fetching: %v", err)
		return
	}
	defer resp.Body.Close()

	b, _ := ioutil.ReadAll(resp.Body)

	var val map[string]interface{}
	if err := json.Unmarshal(b, &val); err != nil {
		app.Log("reminder", "Error parsing: %v", err)
		return
	}

	// Save full JSON data
	data.SaveFile("reminder.json", string(b))

	verseText := fmt.Sprintf("%v", val["verse"])
	// Deduplicate header when Arabic and English names match
	// e.g. "Muhammad - Muhammad - 47:1" → "Muhammad - 47:1"
	verseText = deduplicateVerseName(verseText)
	// Card body is just the verse; the home card framework appends its own
	// "More" link to /islam, so a second link here would be redundant.
	html := fmt.Sprintf(`<div class="item"><div class="verse">%s</div></div>`, verseText)

	reminderMutex.Lock()
	reminderHTML = html
	data.SaveFile("reminder.html", html)
	reminderMutex.Unlock()
	event.Publish(event.Event{Type: "reminder_updated"})

	// Extract message and updated for indexing
	message := stringField(val, "message")
	updated := stringField(val, "updated")

	// Index with just the message summary. The full content (verse, hadith, name)
	// contains markdown that doesn't render well in chat threads, and it changes
	// hourly so embedding it causes stale content.
	summary := message
	if summary == "" {
		summary = "Today's Islamic reminder is ready."
	}

	// Index with ID "daily" (not "reminder_daily") because the chat room type extraction
	// will split "reminder_daily" into type="reminder" and id="daily", then look up just "daily"
	data.Index(
		"daily",
		"reminder",
		"Daily Reminder",
		summary,
		map[string]interface{}{
			"url":     "https://reminder.dev",
			"updated": updated,
			"source":  "daily",
		},
	)

	app.Log("reminder", "Updated reminder")
}

func stringField(val map[string]interface{}, key string) string {
	if s, ok := val[key].(string); ok {
		return s
	}
	return ""
}

// ReminderHTML returns the rendered reminder card HTML
func ReminderHTML() string {
	reminderMutex.RLock()
	defer reminderMutex.RUnlock()
	return reminderHTML
}

// ReminderData represents the cached reminder data
type ReminderData struct {
	Verse   string                 `json:"verse"`
	Name    string                 `json:"name"`
	Hadith  string                 `json:"hadith"`
	Message string                 `json:"message"`
	Updated string                 `json:"updated"`
	Links   map[string]interface{} `json:"links"`
}

// Handler serves /islam in-app: today's full reminder — verse, name of Allah,
// hadith and reflection — rather than bouncing out to reminder.dev. JSON on
// request returns the complete payload.
func Handler(w http.ResponseWriter, r *http.Request) {
	// Prayer times need a location, which only the browser knows. ?lat&lon
	// returns just the timings as JSON; the page fetches it after asking for
	// geolocation, the same way the weather card does.
	if r.URL.Query().Get("lat") != "" {
		lat, err1 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
		lon, err2 := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
		if err1 != nil || err2 != nil {
			app.RespondError(w, http.StatusBadRequest, "Invalid coordinates")
			return
		}
		bearing := QiblaBearing(lat, lon)
		payload := map[string]any{
			"qibla": map[string]any{
				"bearing":  math.Round(bearing*10) / 10,
				"point":    CompassPoint(bearing),
				"distance": math.Round(DistanceToMeccaKm(lat, lon)),
			},
		}
		// Prayer times need an upstream call; the qibla is pure maths, so a
		// timings outage should still leave the compass working.
		if pt, err := GetPrayerTimes(lat, lon, r.URL.Query().Get("tz")); err == nil {
			next, at := pt.Next(time.Now())
			payload["times"] = pt
			payload["next"] = next
			payload["next_at"] = at
		}
		app.RespondJSON(w, payload)
		return
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, GetReminderData())
		return
	}
	app.Respond(w, r, app.Response{
		Title:       "Islam",
		Description: "Prayer times, a daily verse, name of Allah, hadith and reflection",
		HTML:        prayerTimesHTML() + renderIslamPage(GetReminderData()),
	})
}

// prayerTimesHTML renders the prayer-times card. Times are location-specific,
// so the card asks the browser for coordinates (cached in localStorage, shared
// with the weather card) and fills itself in. Without permission it stays a
// quiet prompt rather than an error.
func prayerTimesHTML() string {
	return `<div class="card" id="prayer-card">
  <h3>Prayer times</h3>
  <div id="prayer-body"><p class="text-muted" style="margin:0;font-size:14px">Loading…</p></div>
</div>
<script>
(function(){
  var body=document.getElementById('prayer-body');
  if(!body)return;
  var KEY_LAT='mu_weather_lat',KEY_LON='mu_weather_lon';
  function render(d){
    var t=d.times||{};
    var rows=[['Fajr',t.fajr],['Sunrise',t.sunrise],['Dhuhr',t.dhuhr],['Asr',t.asr],['Maghrib',t.maghrib],['Isha',t.isha]];
    var h='';
    if(d.next){h+='<p style="margin:0 0 10px;font-size:14px">Next: <strong>'+d.next+'</strong> at '+d.next_at+'</p>';}
    h+='<table style="width:100%;font-size:14px;border-collapse:collapse">';
    rows.forEach(function(r){
      if(!r[1])return;
      var isNext=(r[0]===d.next);
      h+='<tr><td style="padding:4px 0;color:'+(isNext?'#111':'#666')+';font-weight:'+(isNext?'600':'400')+'">'+r[0]+'</td>'+
         '<td style="padding:4px 0;text-align:right;font-weight:'+(isNext?'600':'400')+'">'+r[1]+'</td></tr>';
    });
    h+='</table>';
    if(t.date){h+='<p style="margin:10px 0 0;font-size:12px;color:#999">'+t.date+'</p>';}
    if(!t.fajr){h='<p class="text-muted" style="margin:0 0 10px;font-size:14px">Prayer times unavailable right now.</p>';}
    if(d.qibla){h+=qiblaHTML(d.qibla);}
    body.innerHTML=h;
    if(d.qibla){placeMarks(d.qibla.bearing,0);startCompass(d.qibla.bearing);}
  }
  function qiblaHTML(q){
    return '<div style="margin-top:16px;padding-top:14px;border-top:1px solid #eee">'+
      '<p style="margin:0 0 10px;font-size:14px">Qibla: <strong>'+q.bearing+'\u00B0 '+q.point+'</strong>'+
      ' <span style="color:#999">\u00B7 '+q.distance+'km to Mecca</span></p>'+
      '<div style="display:flex;align-items:center;gap:14px">'+
      '<svg id="qibla-dial" width="96" height="96" viewBox="0 0 96 96" style="flex:0 0 auto">'+
        '<circle cx="48" cy="48" r="43" fill="none" stroke="#e0e0e0" stroke-width="1.5"/>'+
        '<g id="qibla-needle" transform="rotate('+q.bearing+' 48 48)">'+
          '<line x1="48" y1="48" x2="48" y2="26" stroke="#111" stroke-width="2" stroke-linecap="round"/>'+
          '<polygon points="48,20 43,30 53,30" fill="#111"/>'+
        '</g>'+
        '<circle cx="48" cy="48" r="2.5" fill="#111"/>'+
        '<text id="qibla-q" text-anchor="middle" font-size="11" font-weight="700" fill="#111">Q</text>'+
        '<text id="qibla-n" text-anchor="middle" font-size="10" fill="#bbb">N</text>'+
      '</svg>'+
      '<p id="qibla-hint" style="margin:0;font-size:12px;color:#999;line-height:1.5">'+
        'Q marks the qibla, N is true north. Hold your phone flat and turn until the needle points up.</p>'+
      '</div></div>';
  }
  // Place the Q and N markers on the rim by angle. They are positioned rather
  // than rotated so the letters always read upright, and N shows where north
  // actually is — on a live compass the top of the dial is the way you are
  // facing, not north.
  function placeMarks(qAngle,nAngle){
    var set=function(id,ang){
      var el=document.getElementById(id);
      if(!el)return;
      var r=ang*Math.PI/180;
      el.setAttribute('x',(48+37*Math.sin(r)).toFixed(1));
      el.setAttribute('y',(48-37*Math.cos(r)+3.6).toFixed(1));
    };
    set('qibla-q',qAngle);
    set('qibla-n',nAngle);
  }
  // Where the device reports its heading, rotate the dial so the needle points
  // at the qibla in the real world rather than just showing a fixed bearing.
  // Rotate the dial so the needle points at the qibla in the real world.
  // Only absolute headings are usable: plain deviceorientation alpha is
  // relative to wherever the device started, not to north.
  function startCompass(bearing){
    var needle=document.getElementById('qibla-needle');
    var hint=document.getElementById('qibla-hint');
    if(!needle||!window.DeviceOrientationEvent)return;
    var smoothed=null,pending=null,frame=null;
    function draw(){
      frame=null;
      if(pending==null)return;
      // Low-pass filter across the 0/360 wrap, so sensor noise doesn't jitter
      // the needle. Without this the dial visibly shakes when you move.
      if(smoothed==null){smoothed=pending;}
      else{
        var d=((pending-smoothed+540)%360)-180;
        smoothed=(smoothed+d*0.18+360)%360;
      }
      var qAngle=(bearing-smoothed+360)%360;
      needle.setAttribute('transform','rotate('+qAngle.toFixed(1)+' 48 48)');
      placeMarks(qAngle,(360-smoothed)%360);
      if(hint){hint.textContent='Following your compass \u2014 turn until the needle points up.';}
    }
    function onOrient(e){
      var h=null;
      if(typeof e.webkitCompassHeading==='number'){h=e.webkitCompassHeading;}
      else if(e.absolute===true&&e.alpha!=null){h=360-e.alpha;}
      if(h==null||isNaN(h))return;
      // The heading is of the device; the dial is drawn in screen space, so
      // correct for however the screen is rotated.
      var so=0;
      if(window.screen&&screen.orientation&&typeof screen.orientation.angle==='number'){so=screen.orientation.angle;}
      else if(typeof window.orientation==='number'){so=window.orientation;}
      pending=(h+so+360)%360;
      if(!frame){frame=requestAnimationFrame(draw);}
    }
    function listen(){
      // One listener only. Registering both absolute and relative events makes
      // them alternate with different reference frames, which flickers.
      if('ondeviceorientationabsolute' in window){
        window.addEventListener('deviceorientationabsolute',onOrient,true);
      }else{
        window.addEventListener('deviceorientation',onOrient,true);
      }
    }
    if(typeof DeviceOrientationEvent.requestPermission==='function'){
      // iOS: needs a user gesture, so ask on tap rather than silently failing.
      var dial=document.getElementById('qibla-dial');
      if(dial){
        dial.style.cursor='pointer';
        if(hint){hint.textContent='Tap the dial to use your compass.';}
        dial.addEventListener('click',function(){
          DeviceOrientationEvent.requestPermission().then(function(p){
            if(p==='granted'){listen();}
          }).catch(function(){});
        });
      }
      return;
    }
    listen();
  }
  function load(lat,lon){
    fetch('/islam?lat='+lat+'&lon='+lon+'&tz='+encodeURIComponent(Intl.DateTimeFormat().resolvedOptions().timeZone||''),{headers:{'Accept':'application/json'},credentials:'same-origin'})
      .then(function(r){return r.ok?r.json():null})
      .then(function(d){ if(d&&(d.times||d.qibla)){render(d)} else {body.innerHTML='<p class="text-muted" style="margin:0;font-size:14px">Prayer times unavailable right now.</p>'} })
      .catch(function(){body.innerHTML='<p class="text-muted" style="margin:0;font-size:14px">Prayer times unavailable right now.</p>'});
  }
  var lat=localStorage.getItem(KEY_LAT),lon=localStorage.getItem(KEY_LON);
  if(lat&&lon){load(lat,lon);return}
  if(!navigator.geolocation){
    body.innerHTML='<p class="text-muted" style="margin:0;font-size:14px">Location unavailable, so prayer times can\'t be shown.</p>';return;
  }
  body.innerHTML='<p class="text-muted" style="margin:0;font-size:14px">Allow location to see prayer times for where you are.</p>';
  navigator.geolocation.getCurrentPosition(function(pos){
    var la=pos.coords.latitude.toFixed(4),lo=pos.coords.longitude.toFixed(4);
    localStorage.setItem(KEY_LAT,la);localStorage.setItem(KEY_LON,lo);
    body.innerHTML='<p class="text-muted" style="margin:0;font-size:14px">Loading…</p>';
    load(la,lo);
  },function(){
    body.innerHTML='<p class="text-muted" style="margin:0;font-size:14px">Location declined, so prayer times can\'t be shown.</p>';
  },{timeout:8000});
})();
</script>`
}

// splitTitleBody splits reminder.dev's "Header\n\nBody" fields into a bold
// header and the body text.
func splitTitleBody(s string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(s), "\n\n", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", strings.TrimSpace(s)
}

// renderIslamPage renders the whole reminder payload: verse, name of Allah,
// hadith and reflection, each with a link to its source on reminder.dev.
func renderIslamPage(rd *ReminderData) string {
	if rd == nil {
		return `<div class="card"><p class="text-muted">Today's reminder is loading — check back shortly.</p></div>`
	}
	var b strings.Builder
	section := func(title, content, linkKey, linkLabel string) {
		if strings.TrimSpace(content) == "" {
			return
		}
		head, body := splitTitleBody(content)
		b.WriteString(`<div class="card"><h3>` + title + `</h3>`)
		if head != "" {
			b.WriteString(`<p style="font-weight:600;margin:0 0 6px">` + html.EscapeString(head) + `</p>`)
		}
		b.WriteString(`<p style="white-space:pre-line;margin:0;line-height:1.6">` + html.EscapeString(body) + `</p>`)
		if linkKey != "" && rd.Links != nil {
			if p, ok := rd.Links[linkKey].(string); ok && p != "" {
				b.WriteString(`<p style="margin:10px 0 0"><a href="https://reminder.dev` + html.EscapeString(p) + `" target="_blank">` + linkLabel + ` &rarr;</a></p>`)
			}
		}
		b.WriteString(`</div>`)
	}
	section("Quran", rd.Verse, "verse", "Read in the Quran")
	section("Name of Allah", rd.Name, "name", "The 99 names")
	section("Hadith", rd.Hadith, "hadith", "Read the hadith")
	if strings.TrimSpace(rd.Message) != "" {
		b.WriteString(`<div class="card"><h3>Reflection</h3><p style="margin:0;line-height:1.6">` + html.EscapeString(rd.Message) + `</p></div>`)
	}
	b.WriteString(`<p style="font-size:12px;color:#999">Daily verse, name of Allah, and hadith via <a href="https://reminder.dev">reminder.dev</a>. Ask the agent to look up any Quran verse or hadith.</p>`)
	return b.String()
}

// GetReminderData loads the cached reminder data (from api/latest, rotates hourly)
func GetReminderData() *ReminderData {
	// Load from cache
	b, err := data.LoadFile("reminder.json")
	if err != nil {
		app.Log("reminder", "Error loading reminder data: %v", err)
		return nil
	}

	var reminderData ReminderData
	if err := json.Unmarshal(b, &reminderData); err != nil {
		app.Log("reminder", "Error parsing reminder data: %v", err)
		return nil
	}

	return &reminderData
}

// GetDailyReminderData fetches the fixed daily reminder from reminder.dev/api/daily.
// Unlike GetReminderData (which rotates hourly), this returns the same content
// all day — suitable for seeding social threads and opinion pieces.
// Results are cached per date to avoid repeated API calls.
func GetDailyReminderData() *ReminderData {
	return GetDailyReminderForDate(time.Now().Format("2006-01-02"))
}

// GetDailyReminderForDate fetches the daily reminder for a specific date (YYYY-MM-DD).
// Results are cached per date.
func GetDailyReminderForDate(date string) *ReminderData {
	cacheFile := "reminder_daily_" + date + ".json"

	// Check cache
	b, err := data.LoadFile(cacheFile)
	if err == nil {
		var rd ReminderData
		if json.Unmarshal(b, &rd) == nil {
			return &rd
		}
	}

	// Fetch from reminder.dev/api/daily?date=YYYY-MM-DD
	url := "https://reminder.dev/api/daily"
	if date != "" {
		url += "?date=" + date
	}

	resp, err := http.Get(url)
	if err != nil {
		app.Log("reminder", "Error fetching daily reminder for %s: %v", date, err)
		// Only fall back to latest for today
		if date == time.Now().Format("2006-01-02") {
			return GetReminderData()
		}
		return nil
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	var rd ReminderData
	if err := json.Unmarshal(body, &rd); err != nil {
		app.Log("reminder", "Error parsing daily reminder for %s: %v", date, err)
		return nil
	}

	// Cache
	data.SaveFile(cacheFile, string(body))
	app.Log("reminder", "Fetched daily reminder for %s", date)
	return &rd
}

// deduplicateVerseName fixes the header line when Arabic and English names
// are identical, e.g. "Muhammad - Muhammad - 47:1" → "Muhammad - 47:1"
// or "Luqman - Luqman - 31:3" → "Luqman - 31:3"
func deduplicateVerseName(text string) string {
	// Header is the first line, before any newline
	firstNewline := strings.Index(text, "\n")
	if firstNewline < 0 {
		firstNewline = len(text)
	}
	header := text[:firstNewline]
	rest := text[firstNewline:]

	// Format is "{Arabic} - {English} - {Chapter}:{Verse}"
	parts := strings.SplitN(header, " - ", 3)
	if len(parts) == 3 && strings.EqualFold(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])) {
		header = parts[0] + " - " + parts[2]
	}

	return header + rest
}
