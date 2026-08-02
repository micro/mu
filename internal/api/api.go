package api

// TokenHeader is the header a caller may use instead of Authorization.
const TokenHeader = "X-Micro-Token"

// This file used to hold Endpoints: twenty REST endpoints written out by hand
// with their params and response shapes, plus Register and Markdown to extend
// and print them.
//
// Nothing read it. The /api page's reference came from the tool registry while
// its playground selector came from here, so the selector offered twenty
// endpoints and the page below documented twenty-five, and neither noticed when
// the other changed. Both now read restTools(), derived from the registered
// tools, so the page cannot disagree with itself.
