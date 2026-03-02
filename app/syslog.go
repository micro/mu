package app

import (
	"fmt"
	"sync"
	"time"
)

const sysLogMaxEntries = 500

// SysLogEntry is a single system log line.
type SysLogEntry struct {
	Time    time.Time
	Package string
	Message string
}

var (
	sysLogMu      sync.Mutex
	sysLogEntries []*SysLogEntry
)

// appendSysLog stores a log message in the in-memory ring buffer.
func appendSysLog(pkg, format string, args ...interface{}) {
	entry := &SysLogEntry{
		Time:    time.Now(),
		Package: pkg,
		Message: fmt.Sprintf(format, args...),
	}
	sysLogMu.Lock()
	sysLogEntries = append(sysLogEntries, entry)
	if len(sysLogEntries) > sysLogMaxEntries {
		sysLogEntries = sysLogEntries[len(sysLogEntries)-sysLogMaxEntries:]
	}
	sysLogMu.Unlock()
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
