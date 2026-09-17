package maps

const directionsUI = `<form id="map-directions" class="card form form-inline">
{{csrf}}<input name="from" placeholder="From" aria-label="Starting point" required><button type="button" id="map-start-here">Use my location</button>
<input name="to" placeholder="Destination" aria-label="Destination" required>
<select name="mode" aria-label="Travel mode"><option value="walk">Walk</option><option value="drive">Drive</option><option value="cycle">Cycle</option><option value="transit">Public transport</option></select><button type="submit">Directions</button>
</form><div id="map-directions-result" aria-live="polite"></div>
`
