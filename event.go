package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Natural language parsing
func parseNaturalLanguage(input string, baseTime time.Time) (*Event, error) {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return nil, fmt.Errorf("empty input")
	}

	event := &Event{
		Start: baseTime,
		End:   baseTime.Add(time.Hour),
	}

	// Parse date
	date := baseTime
	datePatterns := []struct {
		pattern *regexp.Regexp
		parse   func(string, time.Time) time.Time
	}{
		{regexp.MustCompile(`\btoday\b`), func(_ string, base time.Time) time.Time { return base }},
		{regexp.MustCompile(`\btomorrow\b`), func(_ string, base time.Time) time.Time { return base.AddDate(0, 0, 1) }},
		{regexp.MustCompile(`\bnext week\b`), func(_ string, base time.Time) time.Time { return base.AddDate(0, 0, 7) }},
		{regexp.MustCompile(`\b(monday|tuesday|wednesday|thursday|friday|saturday|sunday)\b`), parseWeekday},
	}

	for _, dp := range datePatterns {
		if matches := dp.pattern.FindStringSubmatch(input); matches != nil {
			date = dp.parse(matches[0], baseTime)
			input = dp.pattern.ReplaceAllString(input, "")
			break
		}
	}

	// Parse time
	startTime := date
	timePatterns := []struct {
		pattern *regexp.Regexp
		parse   func(string, time.Time) time.Time
	}{
		{regexp.MustCompile(`\b(\d{1,2}):(\d{2})\s*(am|pm)?\b`), parseTime},
		{regexp.MustCompile(`\b(\d{1,2})\s*(am|pm)\b`), parseTimeSimple},
		{regexp.MustCompile(`\b(morning|afternoon|evening|noon|midnight)\b`), parseTimeWord},
	}

	for _, tp := range timePatterns {
		if matches := tp.pattern.FindStringSubmatch(input); matches != nil {
			startTime = tp.parse(matches[0], date)
			input = tp.pattern.ReplaceAllString(input, "")
			break
		}
	}

	// Extract duration
	duration := time.Hour
	if match := regexp.MustCompile(`\b(\d+)\s*(hour|hours|h|minute|minutes|min)\b`).FindStringSubmatch(input); match != nil {
		val, _ := strconv.Atoi(match[1])
		if strings.Contains(match[2], "hour") || match[2] == "h" {
			duration = time.Duration(val) * time.Hour
		} else {
			duration = time.Duration(val) * time.Minute
		}
		input = regexp.MustCompile(`\b(\d+)\s*(hour|hours|h|minute|minutes|min)\b`).ReplaceAllString(input, "")
	}

	event.Start = startTime
	event.End = startTime.Add(duration)

	// Extract summary (everything else, cleaned up)
	event.Summary = strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(input, " "))
	if event.Summary == "" {
		event.Summary = "New Event"
	}

	return event, nil
}

func parseTime(match string, base time.Time) time.Time {
	re := regexp.MustCompile(`(\d{1,2}):(\d{2})\s*(am|pm)?`)
	matches := re.FindStringSubmatch(match)
	if len(matches) < 3 {
		return base
	}

	hour, _ := strconv.Atoi(matches[1])
	min, _ := strconv.Atoi(matches[2])

	if len(matches) > 3 && matches[3] != "" {
		if matches[3] == "pm" && hour != 12 {
			hour += 12
		} else if matches[3] == "am" && hour == 12 {
			hour = 0
		}
	}

	return time.Date(base.Year(), base.Month(), base.Day(), hour, min, 0, 0, base.Location())
}

func parseTimeSimple(match string, base time.Time) time.Time {
	re := regexp.MustCompile(`(\d{1,2})\s*(am|pm)`)
	matches := re.FindStringSubmatch(match)
	if len(matches) < 3 {
		return base
	}

	hour, _ := strconv.Atoi(matches[1])
	if matches[2] == "pm" && hour != 12 {
		hour += 12
	} else if matches[2] == "am" && hour == 12 {
		hour = 0
	}

	return time.Date(base.Year(), base.Month(), base.Day(), hour, 0, 0, 0, base.Location())
}

func parseTimeWord(match string, base time.Time) time.Time {
	switch match {
	case "morning":
		return time.Date(base.Year(), base.Month(), base.Day(), 9, 0, 0, 0, base.Location())
	case "afternoon":
		return time.Date(base.Year(), base.Month(), base.Day(), 14, 0, 0, 0, base.Location())
	case "evening":
		return time.Date(base.Year(), base.Month(), base.Day(), 18, 0, 0, 0, base.Location())
	case "noon":
		return time.Date(base.Year(), base.Month(), base.Day(), 12, 0, 0, 0, base.Location())
	case "midnight":
		return time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, base.Location())
	}
	return base
}

func parseWeekday(match string, base time.Time) time.Time {
	weekdays := map[string]time.Weekday{
		"monday":    time.Monday,
		"tuesday":   time.Tuesday,
		"wednesday": time.Wednesday,
		"thursday":  time.Thursday,
		"friday":    time.Friday,
		"saturday":  time.Saturday,
		"sunday":    time.Sunday,
	}

	targetDay := weekdays[match]
	daysAhead := int(targetDay - base.Weekday())
	if daysAhead <= 0 {
		daysAhead += 7
	}
	return base.AddDate(0, 0, daysAhead)
}
