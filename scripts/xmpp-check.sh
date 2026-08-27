#!/usr/bin/env bash
#
# Check that an instance's XMPP door actually works, end to end.
#
# A stream header coming back proves the socket is joined up and nothing else.
# What an operator needs to know is whether a real client can log in, whether
# the agent answers, and whether what was said reached the record — three
# things that fail independently and two of which look identical from outside.
#
# Nothing here is Mu-specific except the last check: this is the handshake any
# XMPP client makes, written out so the failure says which step.
#
# Usage:
#   scripts/xmpp-check.sh --host micro.mu --user asim --token <token>
#
#   --port 5223     the direct-TLS port a proxy answers on (default)
#   --plain 5222    talk to Mu directly with no TLS, for running on the server
#                   itself when you are working out whether nginx is the problem
#   --no-agent      skip the model call, which is the slow part
#   --api URL       where the HTTP API is, if not https://<host> — for checking
#                   a local instance, or one behind a proxy on another port
#
# The token is an access token from /token — the same credential IMAP takes.
# Pass it on the command line or in MU_TOKEN; it is never written to a file.

set -uo pipefail

HOST=""; USER_ID=""; TOKEN="${MU_TOKEN:-}"; PORT=5223; PLAIN=""; AGENT=1; API=""

while [ $# -gt 0 ]; do
	case "$1" in
		--host)  HOST="$2"; shift 2 ;;
		--user)  USER_ID="$2"; shift 2 ;;
		--token) TOKEN="$2"; shift 2 ;;
		--port)  PORT="$2"; shift 2 ;;
		--plain) PLAIN="${2:-5222}"; shift 2 ;;
		--no-agent) AGENT=0; shift ;;
		--api)   API="$2"; shift 2 ;;
		-h|--help) sed -n '2,25p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
		*) echo "unknown option: $1" >&2; exit 2 ;;
	esac
done

if [ -z "$HOST" ] || [ -z "$USER_ID" ] || [ -z "$TOKEN" ]; then
	echo "need --host, --user and --token (or MU_TOKEN)" >&2
	exit 2
fi

# An access token is 32 random bytes in unpadded base64url, so it is always
# exactly this long. Checked before anything connects, because a token that
# picked up a stray character on the way through a terminal fails at SASL and
# reads as a protocol problem — three checks pass, everything after sign-in
# fails, and none of it is about XMPP.
#
# A warning rather than a refusal: the length is a property of today's token
# format, and a check that outlived the format should not stop anybody working.
if [ ${#TOKEN} -ne 43 ]; then
	printf '\n  note: that token is %d characters and they are normally 43.\n' "${#TOKEN}"
	printf '        If sign-in fails below, re-copy it from /token rather than\n'
	printf '        looking for a protocol fault.\n'
fi

pass=0; fail=0
ok()   { printf '  \033[32mok\033[0m    %s\n' "$1"; pass=$((pass+1)); }
bad()  { printf '  \033[31mFAIL\033[0m  %s\n' "$1"; fail=$((fail+1)); }
note() { printf '        %s\n' "$1"; }

# run_to bounds a command, because a chat connection is idle by design: the
# server holds it open for ten minutes and a test that waits for the peer to
# hang up waits ten minutes. GNU timeout where there is one, a background kill
# where there is not — macOS ships neither timeout nor gtimeout by default.
run_to() {
	local secs="$1"; shift
	if command -v timeout >/dev/null 2>&1; then
		timeout "$secs" "$@"
	elif command -v gtimeout >/dev/null 2>&1; then
		gtimeout "$secs" "$@"
	else
		"$@" & local p=$!
		( sleep "$secs"; kill -9 "$p" 2>/dev/null ) 2>/dev/null &
		local k=$!
		wait "$p" 2>/dev/null
		kill -9 "$k" 2>/dev/null
	fi
}

OPEN="<?xml version='1.0'?><stream:stream to='$HOST' xmlns='jabber:client' xmlns:stream='http://etherx.jabber.org/streams' version='1.0'>"

# SASL PLAIN is authzid\0authcid\0password. The authzid is empty: it is who the
# client claims to act as, and the server ignores it — the token says who you
# are, and honouring it would be letting the client choose.
AUTH=$(printf '\0%s\0%s' "$USER_ID" "$TOKEN" | base64 | tr -d '\n')

# A word that will not appear by accident, so "the agent replied" is not
# satisfied by an error stanza that happens to contain the right JID.
MARK="xmppcheck$$"

wait_for_agent=25
[ "$AGENT" = 0 ] && wait_for_agent=2

# The whole conversation, sent with pauses so each reply lands before the next
# thing goes out. Captured in one transcript and asserted over afterwards,
# rather than read step by step: a stanza is not a packet, and a reader that
# assumes one read per reply sees half a handshake.
conversation() {
	printf '%s' "$OPEN";                              sleep 1
	printf "<auth xmlns='urn:ietf:params:xml:ns:xmpp-sasl' mechanism='PLAIN'>%s</auth>" "$AUTH"
	sleep 1
	# The RFC requires a new stream after SASL: authentication changes what the
	# stream is, so the client restarts it.
	printf '%s' "$OPEN";                              sleep 1
	printf "<iq type='set' id='b1'><bind xmlns='urn:ietf:params:xml:ns:xmpp-bind'><resource>check</resource></bind></iq>"
	sleep 1
	printf "<iq type='get' id='r1'><query xmlns='jabber:iq:roster'/></iq>"
	sleep 1
	printf "<presence/>";                             sleep 1
	# Two addresses that must be refused, and refused differently. A typo and
	# an unreachable server are different problems with different fixes.
	printf "<message type='chat' to='nobody-here-at-all@%s'><body>x</body></message>" "$HOST"
	sleep 1
	printf "<message type='chat' to='somebody@elsewhere.invalid'><body>x</body></message>"
	sleep 1
	# A note to self. Ordinary — it is how somebody moves a link between their
	# own devices — and it is the cheapest proof that delivery between people
	# works and that what was said reached the record, neither of which needs a
	# model configured.
	printf "<message type='chat' to='%s@%s'><body>%s note to self</body></message>" "$USER_ID" "$HOST" "$MARK"
	sleep 2
	if [ "$AGENT" = 1 ]; then
		printf "<message type='chat' to='agent@%s'><body>Reply with exactly this word and nothing else: %s</body></message>" "$HOST" "$MARK"
	fi
	sleep "$wait_for_agent"
}

echo
if [ -n "$PLAIN" ]; then
	echo "XMPP check — $USER_ID@$HOST via plaintext 127.0.0.1:$PLAIN (no TLS)"
	if ! command -v nc >/dev/null 2>&1; then
		echo "  --plain needs nc" >&2; exit 2
	fi
	OUT=$(conversation | run_to $((wait_for_agent + 15)) nc 127.0.0.1 "$PLAIN" 2>/dev/null)
else
	echo "XMPP check — $USER_ID@$HOST via direct TLS on $PORT"
	OUT=$(conversation | run_to $((wait_for_agent + 15)) \
		openssl s_client -connect "$HOST:$PORT" -servername "$HOST" -quiet -ign_eof 2>/dev/null)
fi
echo

if [ -z "$OUT" ]; then
	bad "nothing came back at all"
	if [ -z "$PLAIN" ]; then
		note "TLS may be terminating and the proxy reaching nothing behind it."
		note "On the server: ss -lntp | grep -E '5222|5223'"
		note "Then re-run with --plain 5222 to cut the proxy out."
	else
		note "Mu is not listening there. Check the log for 'Starting XMPP server'."
	fi
	echo; exit 1
fi

has() { case "$OUT" in *"$1"*) return 0 ;; *) return 1 ;; esac; }

# 1. The stream.
if has "<stream:stream"; then ok "stream opened"; else bad "no stream header"; fi

# 2. The domain. This is the one that silently ruins everything downstream: an
# instance with no MU_DOMAIN or MAIL_DOMAIN calls itself localhost, every JID is
# wrong, and no client can log in to an address that does not exist.
if has "from='$HOST'"; then
	ok "server calls itself $HOST"
else
	bad "server does not call itself $HOST"
	note "Set MU_DOMAIN (or MAIL_DOMAIN) and restart, or every address is wrong."
fi

# 3. Something to log in with.
if has "<mechanism>PLAIN</mechanism>"; then ok "SASL PLAIN offered"; else bad "no SASL mechanism offered"; fi

# 4. The login.
if has "<success"; then
	ok "signed in with an access token"
elif has "not-authorized"; then
	bad "sign-in refused"
	note "The username must be the account id ($USER_ID), not a display name,"
	note "and the token must belong to that account. Mint one at /token."
else
	bad "sign-in neither succeeded nor was refused"
fi

# 5. Addressable.
if has "<jid>$USER_ID@$HOST/check</jid>"; then
	ok "bound as $USER_ID@$HOST/check"
else
	bad "resource bind did not return the full JID"
fi

# 6. Somebody to talk to.
if has "agent@$HOST"; then ok "the agent is in the roster"; else bad "roster does not offer the agent"; fi

# 7 and 8. The two refusals, which must differ.
if has "item-not-found"; then
	ok "an address here that is nobody is refused as a typo"
else
	bad "no item-not-found for an unknown local address"
fi
if has "remote-server-not-found"; then
	ok "another domain is refused as unreachable"
else
	bad "no remote-server-not-found for a foreign domain"
fi

# 9. Delivery between people, without involving a model.
if has "$MARK note to self"; then
	ok "a message to my own address came back to me"
else
	bad "a note to self was not delivered"
	note "Person-to-person routing is broken even if the agent works."
fi

# 10. The agent.
if [ "$AGENT" = 1 ]; then
	if has "$MARK" && has "from='agent@$HOST'"; then
		ok "the agent answered"
	elif has "from='agent@$HOST'"; then
		ok "the agent answered (not with the word it was asked for, which is the model's business)"
	else
		bad "the agent did not answer within ${wait_for_agent}s"
		note "Everything above can pass with no model configured. Check /admin/config,"
		note "and the log for 'chat' lines. Raise the wait if the instance is slow."
	fi
fi

# 11. The record. The point of serving XMPP from this product rather than
# running a chat server: what was said is in the same place mail and the web
# put it, so it is at /inbox afterwards. Over HTTPS, because that is a
# different door and this is checking they meet.
if command -v curl >/dev/null 2>&1; then
	REC=$(curl -fsS --max-time 20 -H "Authorization: Bearer $TOKEN" \
		"${API:-https://$HOST}/api/v1/recall/list" 2>/dev/null)
	# recall renders its answer for a reader rather than returning rows, so the
	# conversation shows as "(chat, <date>)". Matched loosely on purpose: this
	# is checking that the record has the exchange, not what recall's output
	# looks like this month.
	case "$REC" in
		*"(chat,"*|*'"client":"chat"'*|*'"client": "chat"'*)
			ok "the conversation reached the record — it will be at /inbox" ;;
		"")  bad "could not read the record back over the API"
		     note "The same token over HTTPS. If sign-in above also failed, this is"
		     note "one bad credential rather than two broken things." ;;
		*)   bad "nothing filed under the chat client in the record"
		     note "The stanza was delivered but not written down, so /inbox will not show it." ;;
	esac
fi

echo
printf '  %d passed, %d failed\n\n' "$pass" "$fail"
[ "$fail" -eq 0 ] || exit 1
