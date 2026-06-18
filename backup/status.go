package backup

import (
	"bufio"
	"errors"
	"os"
	"strings"
	"time"
)

// LastRunInfo summarizes the most recent recorded backup run, parsed from the
// SYNC SUMMARY block that Upload appends to the log file.
type LastRunInfo struct {
	Time   time.Time // when the run finished
	Status string    // "SUCCESS" or "PARTIAL"
}

// summaryMarker prefixes the "SYNC SUMMARY — <timestamp>" line written by
// summaryBlock; LastRun keys off it. The em dash matches the writer exactly.
const summaryMarker = "SYNC SUMMARY — "

// LastRun scans logPath for the most recent SYNC SUMMARY block and returns its
// timestamp and status. ok is false when the log does not exist yet or holds no
// completed run. A read error (other than a missing file) is returned.
func LastRun(logPath string) (info LastRunInfo, ok bool, err error) {
	if logPath == "" {
		return LastRunInfo{}, false, nil
	}
	f, err := os.Open(logPath)
	if errors.Is(err, os.ErrNotExist) {
		return LastRunInfo{}, false, nil
	}
	if err != nil {
		return LastRunInfo{}, false, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, summaryMarker); i >= 0 {
			ts := strings.TrimSpace(line[i+len(summaryMarker):])
			if t, perr := time.ParseInLocation("2006-01-02 15:04:05", ts, time.Local); perr == nil {
				info.Time = t
				info.Status = "" // status follows on a later line in this block
				ok = true
			}
			continue
		}
		// The "Status : <value>" line sits a few rows below its summary marker;
		// pair it with the marker we just saw.
		if ok && info.Status == "" && strings.HasPrefix(strings.TrimSpace(line), "Status") {
			if c := strings.Index(line, ":"); c >= 0 {
				info.Status = strings.TrimSpace(line[c+1:])
			}
		}
	}
	if scErr := sc.Err(); scErr != nil {
		return LastRunInfo{}, false, scErr
	}
	return info, ok, nil
}
