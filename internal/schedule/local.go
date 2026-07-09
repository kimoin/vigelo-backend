package schedule

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kimoin/vigelo-backend/internal/domain"
	"github.com/kimoin/vigelo-backend/internal/vnmsclient"
)

var (
	ErrInvalidLocalWindow = errors.New("invalid monitored window")
	ErrTooManyUTCWindows  = errors.New("local windows map to more than two UTC windows; simplify the schedule")
)

// ValidateLocalWindows checks up to two non-overlapping whole-hour local windows.
func ValidateLocalWindows(windows []domain.MonitoredWindow) error {
	if len(windows) > 2 {
		return errors.New("you can add up to two monitored windows")
	}
	seen := make([]bool, 24)
	for _, w := range windows {
		start, err := parseHour(w.StartTime)
		if err != nil {
			return err
		}
		end, err := parseHour(w.EndTime)
		if err != nil {
			return err
		}
		duration := (end - start + 24) % 24
		if duration == 0 {
			duration = 24
		}
		if duration == 24 && len(windows) > 1 {
			return errors.New("a full-day window cannot be combined with another window")
		}
		for i := 0; i < duration; i++ {
			h := (start + i) % 24
			if seen[h] {
				return errors.New("monitored windows cannot overlap")
			}
			seen[h] = true
		}
	}
	return nil
}

// LocalToUTC converts household-local windows to VNMS UTC policy for the given day.
func LocalToUTC(loc *time.Location, ref time.Time, windows []domain.MonitoredWindow) ([]vnmsclient.Window, error) {
	if err := ValidateLocalWindows(windows); err != nil {
		return nil, err
	}
	if len(windows) == 0 {
		return []vnmsclient.Window{}, nil
	}
	refLocal := time.Date(ref.Year(), ref.Month(), ref.Day(), 0, 0, 0, 0, loc)
	covered := [24]bool{}
	for _, w := range windows {
		start, _ := parseHour(w.StartTime)
		end, _ := parseHour(w.EndTime)
		duration := (end - start + 24) % 24
		if duration == 0 {
			duration = 24
		}
		for i := 0; i < duration; i++ {
			localHour := (start + i) % 24
			t := refLocal.Add(time.Duration(localHour) * time.Hour)
			covered[t.UTC().Hour()] = true
		}
	}
	return compressUTCHours(covered)
}

// UTCMatchesIntent reports whether VNMS windows match the local intent conversion.
func UTCMatchesIntent(loc *time.Location, ref time.Time, local []domain.MonitoredWindow, vnms []vnmsclient.Window) bool {
	want, err := LocalToUTC(loc, ref, local)
	if err != nil {
		return false
	}
	return windowsEqual(want, vnms)
}

func compressUTCHours(covered [24]bool) ([]vnmsclient.Window, error) {
	n := 0
	for i := 0; i < 24; i++ {
		if covered[i] {
			n++
		}
	}
	if n == 0 {
		return []vnmsclient.Window{}, nil
	}
	if n == 24 {
		return []vnmsclient.Window{{StartHour: 0, DurationHours: 24}}, nil
	}

	var hours []int
	for i := 0; i < 24; i++ {
		if covered[i] {
			hours = append(hours, i)
		}
	}
	runs := contiguousRuns(hours)
	if len(runs) > 2 {
		return nil, ErrTooManyUTCWindows
	}
	out := make([]vnmsclient.Window, 0, len(runs))
	for _, r := range runs {
		out = append(out, vnmsclient.Window{StartHour: r[0], DurationHours: r[1]})
	}
	return out, nil
}

func contiguousRuns(hours []int) [][2]int {
	if len(hours) == 0 {
		return nil
	}
	sort.Ints(hours)
	var runs [][2]int
	start := hours[0]
	prev := hours[0]
	for i := 1; i < len(hours); i++ {
		if hours[i] == prev+1 {
			prev = hours[i]
			continue
		}
		runs = append(runs, [2]int{start, prev - start + 1})
		start = hours[i]
		prev = hours[i]
	}
	runs = append(runs, [2]int{start, prev - start + 1})

	if len(runs) >= 2 {
		first := runs[0]
		last := runs[len(runs)-1]
		if last[0]+last[1] == 24 && first[0] == 0 {
			merged := [2]int{last[0], last[1] + first[1]}
			runs = append(runs[:len(runs)-1], merged)
			runs = runs[1:]
		}
	}
	return runs
}

func windowsEqual(a, b []vnmsclient.Window) bool {
	if len(a) != len(b) {
		return false
	}
	normalize := func(in []vnmsclient.Window) []vnmsclient.Window {
		out := append([]vnmsclient.Window(nil), in...)
		sort.Slice(out, func(i, j int) bool {
			if out[i].StartHour != out[j].StartHour {
				return out[i].StartHour < out[j].StartHour
			}
			return out[i].DurationHours < out[j].DurationHours
		})
		return out
	}
	na, nb := normalize(a), normalize(b)
	for i := range na {
		if na[i] != nb[i] {
			return false
		}
	}
	return true
}

func parseHour(s string) (int, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 || parts[1] != "00" {
		return 0, fmt.Errorf("%w: use whole-hour 24-hour times like 08:00", ErrInvalidLocalWindow)
	}
	var h int
	if _, err := fmt.Sscanf(parts[0], "%02d", &h); err != nil || h < 0 || h > 24 {
		return 0, fmt.Errorf("%w: use whole-hour 24-hour times like 08:00", ErrInvalidLocalWindow)
	}
	if h == 24 {
		h = 0
	}
	return h, nil
}

func LoadLocation(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Europe/Helsinki"
	}
	return time.LoadLocation(name)
}
