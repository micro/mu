package prayer

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
	"mu/internal/auth"
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
	if err := service.Register(Spec); err != nil {
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
	// "More" link to /prayer, so a second link here would be redundant.
	html := fmt.Sprintf(`<div class="item"><div class="verse">%s</div></div>`, verseText)

	reminderMutex.Lock()
	reminderHTML = html
	data.SaveFile("reminder.html", html)
	reminderMutex.Unlock()
	event.Publish(event.Event{Type: "reminder_updated"})

	// Extract message and updated for indexing
	message := stringField(val, "message")
	updated := stringField(val, "updated")

	// Index with just the message summary. The full content (verse, saying, name)
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

	// And the reflection itself, kept.
	//
	// The row above is one row. Its id is the constant "daily" and the index is
	// keyed on id, so every fetch overwrites the last — hourly, forever. What
	// survived was the message summary of whatever arrived most recently, which
	// is the right thing for a card that says what today is and no use at all
	// to somebody asking what they read last Ramadan.
	//
	// So a second entry, dated, holding the whole thing: the verse, the saying,
	// the name and the message. This is the part with something in it. A verse
	// somebody read in March is not stale in June the way a headline is — it is
	// the same verse, and being able to find it again is most of why reading it
	// was worth anything.
	//
	// Keyed on the reflection's own updated stamp, which is what makes one row
	// one reflection. These arrive hourly, not daily — keying on the date would
	// collapse twenty-four of them into whichever came last, which is the same
	// bug as the constant id above with a longer period. And keying on the time
	// we fetched would do the opposite: two fetches of an unchanged reflection
	// would be two rows. The publisher's stamp is the only clock that means
	// "this is a different reflection".
	if key := reflectionKey(updated); key != "" {
		data.Index(
			"reminder-"+key,
			"reminder",
			reflectionTitle(val, updated),
			reflectionText(val),
			reflectionMeta(val, updated),
		)
	}

	app.Log("reminder", "Updated reminder")
}

// reflectionKey identifies one reflection, from the stamp the publisher put on
// it, normalised to UTC so the same moment is the same key whatever offset it
// arrived in.
//
// To the second rather than to the hour or the day. Reflections are published
// hourly and there is no promise they are exactly on the hour; rounding is a
// guess about somebody else's schedule, and the cost of guessing wrong is
// losing one.
//
// An unreadable stamp falls back to the hour we saw it, which keeps a refetch
// within the same hour from writing a second row. That is a worse key and it
// is only reached when the payload has no usable one — losing the reflection
// entirely would be worse still.
func reflectionKey(updated string) string {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05Z", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, strings.TrimSpace(updated)); err == nil {
			return t.UTC().Format("2006-01-02T15:04:05Z")
		}
	}
	return time.Now().UTC().Format("2006-01-02T15Z")
}

// reflectionTitle is what the archive shows in a list.
//
// The verse's reference, because that is what somebody scans for — a column of
// "Reflection — 2026-08-30T12:14:08Z" is a column of timestamps, and the thing
// being looked for is a verse. The stamp is in the metadata either way.
func reflectionTitle(val map[string]interface{}, updated string) string {
	verse := deduplicateVerseName(strings.TrimSpace(stringField(val, "verse")))
	if line := strings.TrimSpace(firstLine(verse)); line != "" {
		return line
	}
	if k := reflectionKey(updated); k != "" {
		return "Reflection — " + k
	}
	return "Reflection"
}

// reflectionMeta carries the stamp and the publisher's own deep links, so an
// entry found in the archive can be followed back to the verse, the hadith and
// the name it came from rather than to the site's front page.
func reflectionMeta(val map[string]interface{}, updated string) map[string]interface{} {
	meta := map[string]interface{}{
		"url":     "https://reminder.dev",
		"updated": updated,
		"source":  "reflection",
	}
	links, _ := val["links"].(map[string]interface{})
	for _, k := range []string{"verse", "hadith", "name"} {
		if v, ok := links[k].(string); ok && strings.TrimSpace(v) != "" {
			meta[k+"_url"] = "https://reminder.dev" + v
		}
	}
	return meta
}

// firstLine is the text up to the first newline, for a title.
func firstLine(s string) string {
	if i := strings.IndexAny(s, "\n\r"); i >= 0 {
		return s[:i]
	}
	return s
}

// reflectionText is the whole reflection as one searchable body.
//
// Labelled, because the parts are different kinds of thing and somebody
// searching for a hadith should not get a verse. Markdown is left as it is:
// the reason the summary was indexed alone was that markdown renders badly in
// a chat thread, which is an argument about one reader rather than about what
// is worth keeping.
func reflectionText(val map[string]interface{}) string {
	var b strings.Builder
	for _, part := range []struct{ label, key string }{
		{"Verse", "verse"},
		{"Hadith", "hadith"},
		{"Name", "name"},
		{"Reflection", "message"},
	} {
		if v := strings.TrimSpace(stringField(val, part.key)); v != "" {
			b.WriteString(part.label + ": " + v + "\n\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func stringField(val map[string]interface{}, key string) string {
	if s, ok := val[key].(string); ok {
		return s
	}
	return ""
}

// ReminderHTML returns the rendered reminder card HTML
func ReminderHTML(who service.Viewer) string {
	reminderMutex.RLock()
	body := reminderHTML
	reminderMutex.RUnlock()
	return nextMark(who.Account) + body
}

// nextMark is the next prayer, in the corner of the card.
//
// Computed here when this instance knows where the reader is, which it does now
// for anybody who has set a place — see account/place.go. That is the whole
// difference between a card that answers and one that waits: the mark used to
// come from coordinates a browser had cached, so it was empty on a first visit,
// empty on a second device, and empty in anything that is not a browser at all.
//
// A prayer time is the case that makes this worth doing rather than merely
// tidy. It is not a convenience — it is the reason somebody opens the page, it
// is wrong everywhere but one latitude, and it cannot be guessed.
func nextMark(accountID string) string {
	lat, lon, ok := auth.Located(accountID)
	if !ok {
		return browserMark
	}
	zone := ""
	if acc, err := auth.GetAccount(accountID); err == nil && acc != nil {
		zone = acc.Zone
	}
	times, err := GetPrayerTimes(lat, lon, zone, "")
	if err != nil || times == nil {
		return browserMark
	}
	loc := time.UTC
	if zone != "" {
		if z, err := time.LoadLocation(zone); err == nil {
			loc = z
		}
	}
	name, at := times.Next(time.Now().In(loc))
	if name == "" {
		return ""
	}
	return `<span class="card-corner">` + html.EscapeString(name+" "+at) + `</span>`
}

// browserMark is the fallback for a reader this instance does not have a place
// for: it fills itself in from coordinates a browser cached, and stays empty
// when there are none.
//
// It puts the next prayer in the corner of the home card — "Asr
// 14:25" — so the card answers the time-sensitive question at a glance and the
// verse stays the body of it.
//
// It fills itself in from coordinates the reader has already granted elsewhere
// (the weather and prayer cards share these keys). It never asks for location
// itself: the home screen is not the place to prompt, and with nothing cached
// the mark simply stays empty.
const browserMark = `<span id="prayer-next" class="card-corner"></span>
<script>
(function(){
  var el=document.getElementById('prayer-next');
  if(!el)return;
  var la=null,lo=null,m=null;
  try{la=localStorage.getItem('mu_weather_lat');lo=localStorage.getItem('mu_weather_lon');
      m=localStorage.getItem('mu_prayer_method');}catch(e){}
  if(!la||!lo)return;
  var tz='';try{tz=Intl.DateTimeFormat().resolvedOptions().timeZone||'';}catch(e){}
  var u='/prayer?lat='+encodeURIComponent(la)+'&lon='+encodeURIComponent(lo)+
        '&tz='+encodeURIComponent(tz)+(m?'&method='+encodeURIComponent(m):'');
  fetch(u,{headers:{'Accept':'application/json'}}).then(function(r){return r.json();}).then(function(d){
    if(d&&d.next&&d.next_at){el.textContent=d.next+' '+d.next_at;}
  }).catch(function(){});
})();
</script>`

// ReminderData represents the cached reminder data
type ReminderData struct {
	Verse   string                 `json:"verse"`
	Name    string                 `json:"name"`
	Hadith  string                 `json:"hadith"`
	Message string                 `json:"message"`
	Updated string                 `json:"updated"`
	Links   map[string]interface{} `json:"links"`
}

// Handler serves /prayer in-app: today's full reflection — verse, saying, name
// and message — rather than bouncing out to reminder.dev. JSON on
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
		if pt, err := GetPrayerTimes(lat, lon, r.URL.Query().Get("tz"), r.URL.Query().Get("method")); err == nil {
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
	// Prayer times beside the reminder rather than above it: the times are a
	// short table that does not need the full width, and the verse is what
	// most readers came for. The aside is first in the DOM so the stacked
	// phone layout leads with the times.
	body := `<div class="prayer-layout">` +
		`<aside class="prayer-side">` + prayerTimesHTML() + `</aside>` +
		`<div class="prayer-main">` + renderReflectionPage(GetReminderData()) + `</div>` +
		`</div>`
	app.Respond(w, r, app.Response{
		Title:       "Prayer",
		Description: "Islamic prayer times, the qibla, and a daily verse, saying, name and reflection",
		HTML:        body,
	})
}

// prayerTimesHTML renders the prayer-times card. Times are location-specific,
// so the card asks the browser for coordinates (cached in localStorage, shared
// with the weather card) and fills itself in. Without permission it stays a
// quiet prompt rather than an error.
func prayerTimesHTML() string {
	var opts strings.Builder
	for _, c := range ConventionNames() {
		sel := ""
		if c.ID == DefaultConvention {
			sel = " selected"
		}
		opts.WriteString(`<option value="` + c.ID + `"` + sel + `>` + html.EscapeString(c.Label) + `</option>`)
	}
	return `<div class="card" id="prayer-card">
  <h3>Prayer times</h3>
  <div id="prayer-body"><p class="text-muted m-0 text-base">Loading…</p></div>
  <p class="mt-3 m-0 text-xs text-muted">
    Calculation
    <select id="prayer-method" class="num-field ml-1">` + opts.String() + `</select>
  </p>
</div>
<script>
(function(){
  var body=document.getElementById('prayer-body');
  if(!body)return;
  var KEY_LAT='mu_weather_lat',KEY_LON='mu_weather_lon',KEY_M='mu_prayer_method';
  var sel=document.getElementById('prayer-method');
  // Fajr and Isha depend on the twilight angle, and conventions differ by up to
  // an hour, so remember which one this reader uses.
  var saved=null;try{saved=localStorage.getItem(KEY_M);}catch(e){}
  if(sel&&saved){sel.value=saved;}
  function method(){return (sel&&sel.value)||saved||'';}
  if(sel){sel.addEventListener('change',function(){
    try{localStorage.setItem(KEY_M,sel.value);}catch(e){}
    var la=localStorage.getItem(KEY_LAT),lo=localStorage.getItem(KEY_LON);
    if(la&&lo){load(la,lo);}
  });}
  function render(d){
    var t=d.times||{};
    var rows=[['Fajr',t.fajr],['Sunrise',t.sunrise],['Dhuhr',t.dhuhr],['Asr',t.asr],['Maghrib',t.maghrib],['Isha',t.isha]];
    var h='';
    if(d.next){h+='<p class="m-0 mb-3 text-base">Next: <strong>'+d.next+'</strong> at '+d.next_at+'</p>';}
    h+='<table class="rule-table text-base">';
    rows.forEach(function(r){
      if(!r[1])return;
      var isNext=(r[0]===d.next);
      var cls=isNext?' pr-next':'';
      h+='<tr class="pr-row'+cls+'"><td>'+r[0]+'</td><td class="right">'+r[1]+'</td></tr>';
    });
    h+='</table>';
    if(t.date){h+='<p class="mt-3 m-0 text-xs text-muted">'+t.date+'</p>';}
    if(!t.fajr){h='<p class="text-muted m-0 mb-3 text-base">Prayer times unavailable right now.</p>';}
    if(d.qibla){h+=qiblaHTML(d.qibla);}
    body.innerHTML=h;
    if(d.qibla){placeMarks(d.qibla.bearing,0);startCompass(d.qibla.bearing);}
  }
  function qiblaHTML(q){
    return '<div class="mt-4 top-rule">'+
      '<p class="m-0 mb-3 text-base">Qibla: <strong>'+q.bearing+'\u00B0 '+q.point+'</strong>'+
      ' <span class="text-muted">\u00B7 '+q.distance+'km to Mecca</span></p>'+
      '<div class="d-flex items-center gap-4">'+
      // Ten units of headroom above the dial, for the target and its Q to sit
      // outside the rim where nothing else is. Inside it they landed on the
      // needle's arrowhead, which is where the needle goes when you have got
      // it right — the one moment the dial has to be readable.
      '<svg id="qibla-dial" width="96" height="96" viewBox="0 -10 96 106" class="fixed-w">'+
        '<circle cx="48" cy="48" r="38" fill="none" stroke="#e0e0e0" stroke-width="1.5"/>'+
        // The target, at the top of the rim, with Q above it. Like the mark on
        // the bezel of a real qibla compass: it does not mean a direction in
        // the world, it is the slot you bring the needle into.
        //
        // Hidden until a live heading exists, because without one there is
        // nothing to aim — the dial is then a north-up diagram of a bearing.
        // It stayed hidden with one too: .d-none is display:none !important,
        // and this was revealed by clearing an inline style, which an
        // !important rule beats. So the instruction said "turn until Q reaches
        // the marker at the top" and there was no marker at the top.
        //
        // A dot rather than a tick, because Q sits over it and Q has a tail,
        // which at this size lands in whatever occupies the few units below
        // its baseline. And after the rim rather than before, because the rim
        // is painted over what came first and a light line across the middle
        // of a dark dot reads as half a dot.
        '<circle id="qibla-index" cx="48" cy="10" r="2.5" fill="#111" class="d-none"/>'+
        // Just the needle. Tick marks collided with the N label and added
        // nothing the labels don't already say.
        // Long enough to reach for the rim: a needle that stops two-thirds of
        // the way out does not read as pointing at anything on it.
        '<g id="qibla-needle" transform="rotate('+q.bearing+' 48 48)">'+
          '<line x1="48" y1="48" x2="48" y2="26" stroke="#111" stroke-width="2" stroke-linecap="round"/>'+
          '<polygon points="48,16 43,27 53,27" fill="#111"/>'+
        '</g>'+
        '<circle id="qibla-pivot" cx="48" cy="48" r="2.5" fill="#111"/>'+
        '<text id="qibla-q" text-anchor="middle" font-size="11" font-weight="700" fill="#111">Q</text>'+
        '<text id="qibla-n" text-anchor="middle" font-size="10" fill="#bbb">N</text>'+
      '</svg>'+
      '<p id="qibla-hint" class="m-0 text-xs text-muted lh-15">'+
        'Q marks the qibla, N is true north.</p>'+
      '</div></div>';
  }
  // Place the Q and N letters on the rim by angle. They are positioned rather
  // than rotated so they always read upright, and N shows where north actually
  // is — on a live compass the top of the dial is the way you are facing, not
  // north.
  //
  // What the two letters mean differs by mode, which is why the caller passes
  // both angles rather than working them out here. With no heading this is a
  // north-up diagram: N at the top, Q out at the bearing, needle pointing at
  // it. With a heading it is an instrument: Q is pinned to the target at the
  // top and the needle swings, so lining them up is the whole interaction.
  function setMark(id,ang,radius){
    var el=document.getElementById(id);
    if(!el)return;
    var r=ang*Math.PI/180;
    el.setAttribute('x',(48+radius*Math.sin(r)).toFixed(1));
    el.setAttribute('y',(48-radius*Math.cos(r)+3.6).toFixed(1));
  }
  function placeMarks(qAngle,nAngle){
    setMark('qibla-q',qAngle,37);
    setMark('qibla-n',nAngle,37);
  }
  // pinQ puts Q on the target, outside the rim at the top, where it stays for
  // as long as there is a heading. Further out than the rose letters because
  // it is not one: a target on the bezel, not a direction in the world.
  function pinQ(){setMark('qibla-q',0,52);}
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
      // Q pinned to the target at the top; only the needle moves. It used to
      // ride the needle's tip, which made the two one object — so "turn until
      // Q reaches the marker" could only ever mean "turn until the arrow
      // points up", and there was no marker up there to reach.
      pinQ();
      setMark('qibla-n',(360-smoothed)%360,37);
      // classList, not style.display: .d-none is display:none !important and
      // an inline empty string does not beat it. The marker was never drawn.
      var idx=document.getElementById('qibla-index');
      if(idx){idx.classList.remove('d-none');}
      // And say when you have arrived. "Turn until…" with no arrival is the
      // same instruction whether you are facing Mecca or facing away.
      var off=Math.min(qAngle,360-qAngle);
      var there=off<=5;
      paint(there?'#1a7f37':'#111');
      if(hint){hint.textContent=there?'Facing the qibla.':'Turn until the arrow points at Q.';}
    }
    // Needle, target and Q are one signal, so they take one colour.
    function paint(c){
      var g=document.getElementById('qibla-needle');
      if(g){
        var ls=g.getElementsByTagName('line');
        for(var i=0;i<ls.length;i++){ls[i].setAttribute('stroke',c);}
        var ps=g.getElementsByTagName('polygon');
        for(var j=0;j<ps.length;j++){ps[j].setAttribute('fill',c);}
      }
      var q=document.getElementById('qibla-q');if(q){q.setAttribute('fill',c);}
      var pv=document.getElementById('qibla-pivot');if(pv){pv.setAttribute('fill',c);}
      var ix=document.getElementById('qibla-index');if(ix){ix.setAttribute('fill',c);}
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
    fetch('/prayer?lat='+lat+'&lon='+lon+'&tz='+encodeURIComponent(Intl.DateTimeFormat().resolvedOptions().timeZone||'')+'&method='+encodeURIComponent(method()),{headers:{'Accept':'application/json'},credentials:'same-origin'})
      .then(function(r){return r.ok?r.json():null})
      .then(function(d){ if(d&&(d.times||d.qibla)){render(d)} else {body.innerHTML='<p class="text-muted m-0 text-base">Prayer times unavailable right now.</p>'} })
      .catch(function(){body.innerHTML='<p class="text-muted m-0 text-base">Prayer times unavailable right now.</p>'});
  }
  var lat=localStorage.getItem(KEY_LAT),lon=localStorage.getItem(KEY_LON);
  if(lat&&lon){load(lat,lon);return}
  if(!navigator.geolocation){
    body.innerHTML='<p class="text-muted m-0 text-base">Location unavailable, so prayer times can\'t be shown.</p>';return;
  }
  body.innerHTML='<p class="text-muted m-0 text-base">Allow location to see prayer times for where you are.</p>';
  navigator.geolocation.getCurrentPosition(function(pos){
    var la=pos.coords.latitude.toFixed(4),lo=pos.coords.longitude.toFixed(4);
    localStorage.setItem(KEY_LAT,la);localStorage.setItem(KEY_LON,lo);
    body.innerHTML='<p class="text-muted m-0 text-base">Loading…</p>';
    load(la,lo);
  },function(){
    body.innerHTML='<p class="text-muted m-0 text-base">Location declined, so prayer times can\'t be shown.</p>';
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

// renderReflectionPage renders the whole payload: Verse, Saying, Name and
// Reflection, each with a link to its source on reminder.dev.
//
// The headings are the plain words rather than Quran, hadith and name of Allah.
// The content is unchanged and the links say where each one comes from — a
// reader who knows the tradition loses nothing, and one who does not is not
// asked to know it before they can read a line.
func renderReflectionPage(rd *ReminderData) string {
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
			b.WriteString(`<p class="semibold m-0 mb-2">` + html.EscapeString(head) + `</p>`)
		}
		b.WriteString(`<p class="pre-line m-0">` + html.EscapeString(body) + `</p>`)
		if linkKey != "" && rd.Links != nil {
			if p, ok := rd.Links[linkKey].(string); ok && p != "" {
				b.WriteString(`<p class="mt-3 m-0"><a href="https://reminder.dev` + html.EscapeString(p) + `" target="_blank">` + linkLabel + ` &rarr;</a></p>`)
			}
		}
		b.WriteString(`</div>`)
	}
	section("Verse", rd.Verse, "verse", "Read in the Quran")
	section("Saying", rd.Hadith, "hadith", "Read the hadith")
	section("Name", rd.Name, "name", "The 99 names of Allah")
	if strings.TrimSpace(rd.Message) != "" {
		b.WriteString(`<div class="card"><h3>Reflection</h3><p class="m-0">` + html.EscapeString(rd.Message) + `</p></div>`)
	}
	b.WriteString(`<p class="text-xs text-muted">A daily verse of the Quran, a hadith and a name of Allah, via <a href="https://reminder.dev">reminder.dev</a>. Ask the agent to look up any verse or hadith.</p>`)
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

// DailyReminderData fetches the fixed daily reminder from reminder.dev/api/daily.
// Unlike GetReminderData (which rotates hourly), this returns the same content
// all day — suitable for seeding social threads and opinion pieces.
// Results are cached per date to avoid repeated API calls.
func DailyReminderData() *ReminderData {
	return DailyReminderForDate(time.Now().Format("2006-01-02"))
}

// DailyReminderForDate fetches the daily reminder for a specific date (YYYY-MM-DD).
// Results are cached per date.
func DailyReminderForDate(date string) *ReminderData {
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
