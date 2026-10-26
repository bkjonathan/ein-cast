package portproc

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"strconv"
	"strings"

	"eincast/internal/app"
)

// parseLsof reads `lsof -Fpc` output: a "p<pid>" line starts a process, and the
// "c<command>" line after it names it. A process listening on IPv4 and IPv6 is
// reported once by lsof, but duplicates are dropped anyway.
func parseLsof(out []byte) []app.PortHolder {
	var holders []app.PortHolder
	seen := map[int]bool{}
	cur := -1 // index in holders of the process the next "c" line belongs to
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			pid, err := strconv.Atoi(line[1:])
			if err != nil || seen[pid] {
				cur = -1
				continue
			}
			seen[pid] = true
			holders = append(holders, app.PortHolder{PID: pid})
			cur = len(holders) - 1
		case 'c':
			if cur >= 0 {
				holders[cur].Name = line[1:]
			}
		}
	}
	return holders
}

// parseNetstat reads `netstat -ano -p tcp` output and returns the processes
// listening on port. A listening socket has no peer, so its remote address ends
// in ":0"; that is used instead of the word "LISTENING", which Windows
// translates into the display language.
func parseNetstat(out []byte, port int) []app.PortHolder {
	suffix := ":" + strconv.Itoa(port)
	var holders []app.PortHolder
	seen := map[int]bool{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		// proto, local, remote, state, pid
		if len(f) < 5 || !strings.EqualFold(f[0], "TCP") {
			continue
		}
		if !strings.HasSuffix(f[1], suffix) || !strings.HasSuffix(f[2], ":0") {
			continue
		}
		pid, err := strconv.Atoi(f[len(f)-1])
		if err != nil || pid <= 0 || seen[pid] {
			continue
		}
		seen[pid] = true
		holders = append(holders, app.PortHolder{PID: pid})
	}
	return holders
}

// parseTasklist reads one row of `tasklist /FO CSV /NH`: "image","pid",...
// When no process matched, tasklist prints an INFO sentence instead, which does
// not parse as a row with several columns, so the name stays empty.
func parseTasklist(out []byte) string {
	r := csv.NewReader(bytes.NewReader(out))
	r.FieldsPerRecord = -1
	rec, err := r.Read()
	if err != nil || len(rec) < 2 {
		return ""
	}
	return rec[0]
}
