package app

import (
	"fmt"
	"sync"
	"time"
)

const sysLogMaxEntries = 500

// alertMaxEntries is how many alerts are kept.
//
// A separate list, and that is the whole point of it. Alerts were already being
// recorded — they went through Log like everything else — and were already
// invisible, because five hundred lines of ordinary chatter is an hour or two
// on a busy instance and then the alert is gone. Something that says the key
// store refused a write, or that an answer was given away because settlement
// failed, must still be there tomorrow when somebody asks why.
const alertMaxEntries = 200

// SysLogEntry is a single system log line.
type SysLogEntry struct {
	Time    time.Time
	Package string
	Message string
	// Alert marks a line somebody has to look at. Not an error — errors are
	// ordinary and most of them are somebody else's server being slow. This is
	// for the ones where the instance did something it should not have had to,
	// or refused something to protect itself.
	Alert bool
}

var (
	sysLogMu      sync.Mutex
	sysLogEntries []*SysLogEntry
	alertEntries  []*SysLogEntry
)

// appendSysLog stores a log message in the in-memory ring buffer.
func appendSysLog(pkg, format string, args ...interface{}) {
	appendEntry(pkg, false, format, args...)
}

func appendEntry(pkg string, alert bool, format string, args ...interface{}) {
	entry := &SysLogEntry{
		Time:    time.Now(),
		Package: pkg,
		Message: fmt.Sprintf(format, args...),
		Alert:   alert,
	}
	sysLogMu.Lock()
	sysLogEntries = append(sysLogEntries, entry)
	if len(sysLogEntries) > sysLogMaxEntries {
		sysLogEntries = sysLogEntries[len(sysLogEntries)-sysLogMaxEntries:]
	}
	if alert {
		alertEntries = append(alertEntries, entry)
		if len(alertEntries) > alertMaxEntries {
			alertEntries = alertEntries[len(alertEntries)-alertMaxEntries:]
		}
	}
	sysLogMu.Unlock()
}

// Alert records something somebody has to look at, and logs it like anything
// else.
//
// The bar is deliberately high: the instance did something it should not have
// had to, or refused something in order to protect itself. A provider timing
// out is not an alert. A save refused because it would have destroyed private
// keys is.
//
// It was a convention — the word CRITICAL at the front of an ordinary log line
// — which is a convention nothing can count, filter or show you. Three places
// were using it and none of them was visible anywhere.
func Alert(pkg string, format string, args ...interface{}) {
	logLine(pkg, "ALERT: "+format, args...)
	appendEntry(pkg, true, format, args...)
}

// Alerts returns the retained alerts, newest first.
func Alerts() []*SysLogEntry {
	sysLogMu.Lock()
	defer sysLogMu.Unlock()
	out := make([]*SysLogEntry, len(alertEntries))
	for i, e := range alertEntries {
		out[len(alertEntries)-1-i] = e
	}
	return out
}

// AlertCount is how many are being held, for a badge.
func AlertCount() int {
	sysLogMu.Lock()
	defer sysLogMu.Unlock()
	return len(alertEntries)
}

// GetSysLog returns a copy of the system log in reverse-chronological order.
func GetSysLog() []*SysLogEntry {
	sysLogMu.Lock()
	defer sysLogMu.Unlock()
	result := make([]*SysLogEntry, len(sysLogEntries))
	for i, e := range sysLogEntries {
		result[len(sysLogEntries)-1-i] = e
	}
	return result
}
