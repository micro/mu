# Forms

Forms are private until their owner publishes them. Publishing exposes the
questions and an anonymous submission endpoint. Responses always remain private
to the owner; publishing a form never publishes its responses.

Open **Services → Forms → New form**. Add a title, description and up to twelve
short text, long text or email fields. Mark required answers, enable **Public**,
and save. The form page provides a share link and HTML for another website.
Responses appear on that page. Close submissions or unpublish to stop receiving
new answers; existing responses remain available to the owner.

## External websites

A normal HTML form can POST to `/forms/submit?id=FORM_ID` on the instance hosting
the form. Use the generated HTML so input names match the current definition.
There is no API token or account cookie to put on the external website. Use
`application/x-www-form-urlencoded`; JavaScript is not required. Successful HTML
submissions redirect to a receipt page. `Accept: application/json` returns
`{"status":"received"}`. External pages can submit with JavaScript using
URL-encoded fields, `Accept: application/json` and `credentials: "omit"` to
show confirmation on their own page. Only this anonymous submission endpoint
allows cross-origin reads; management and responses remain private. Normal
HTML submission remains available without JavaScript.

Management and response access use the authenticated `forms` service through
the normal API/MCP interfaces: Write, Read, List, Responses and Delete. List and
Responses return up to 50 items; use offset for subsequent pages. Write replaces
the definition, so include the fields and sharing state you want to retain.

## Boundaries

- The published form determines the response owner. Visitor-supplied ownership,
  visibility and unknown fields are rejected.
- Each response retains its field labels and types at submission time.
- A short answer is limited to 1,000 characters; a long answer to 8,000. The HTTP
  request is limited to 32 KB; stored responses inherit the record store's 64 KB
  limit and 2,000-response cap per form.
- Public submissions are limited to 20 per minute per client IP and 100 per hour
  per form. The bounded in-memory rate counters reset on server restart; the
  storage cap persists. Forwarded IPs follow the instance's trusted-proxy rules.
- Submitted email addresses are unverified. Responses are untrusted content.
- Responses are saved in Forms; they do not trigger email or an agent reply.
- Deleting a form deletes its responses. Account deletion removes all owned
  forms and responses. Public submission never uses a visitor's session.
