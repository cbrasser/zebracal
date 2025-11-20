package main

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/charmbracelet/lipgloss"
)

func loadICSFromReader(reader io.Reader, calendarName string, color lipgloss.Color) ([]Event, error) {
	cal, err := ics.ParseCalendar(reader)
	if err != nil {
		return nil, err
	}

	var events []Event
	now := time.Now()
	// Expand recurring events up to 1 year in the future
	maxDate := now.AddDate(1, 0, 0)

	for _, event := range cal.Events() {
		start, err := event.GetStartAt()
		if err != nil {
			continue
		}

		end, err := event.GetEndAt()
		if err != nil {
			end = start.Add(time.Hour)
		}

		summary := ""
		if summaryProp := event.GetProperty(ics.ComponentPropertySummary); summaryProp != nil {
			summary = summaryProp.Value
		}

		description := ""
		if descProp := event.GetProperty(ics.ComponentPropertyDescription); descProp != nil {
			description = descProp.Value
		}

		uid := ""
		if uidProp := event.GetProperty(ics.ComponentPropertyUniqueId); uidProp != nil {
			uid = uidProp.Value
		}

		if summary == "" {
			summary = "(No title)"
		}

		// Check for RRULE (recurrence rule) - try multiple property access methods
		var rruleValue string
		
		// First, try accessing all properties to find RRULE (most reliable)
		for _, prop := range event.Properties {
			// IANAToken is a field, not a method
			if strings.ToUpper(prop.IANAToken) == "RRULE" {
				rruleValue = prop.Value
				break
			}
		}
		
		// If not found in Properties, try GetProperty with extended
		if rruleValue == "" {
			rruleProp := event.GetProperty(ics.ComponentPropertyExtended("RRULE"))
			if rruleProp != nil {
				rruleValue = rruleProp.Value
			} else {
				// Try with lowercase
				rruleProp = event.GetProperty(ics.ComponentPropertyExtended("rrule"))
				if rruleProp != nil {
					rruleValue = rruleProp.Value
				}
			}
		}

		if rruleValue != "" {
			// Parse RRULE and expand occurrences
			occurrences := expandRecurringEvent(start, end, rruleValue, maxDate, now)
			for _, occ := range occurrences {
				events = append(events, Event{
					Summary:       summary,
					Start:         occ.Start,
					End:           occ.End,
					Description:   description,
					CalendarName:  calendarName,
					CalendarColor: color,
					UID:           uid,
				})
			}
		} else {
			// Single event (non-recurring) - include even if in the past (for today's view)
			events = append(events, Event{
				Summary:       summary,
				Start:         start,
				End:           end,
				Description:   description,
				CalendarName:  calendarName,
				CalendarColor: color,
				UID:           uid,
			})
		}
	}

	return events, nil
}

func loadICSFromURL(url string, calendarName string, color lipgloss.Color) ([]Event, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch calendar: %s", resp.Status)
	}

	return loadICSFromReader(resp.Body, calendarName, color)
}

func loadICSFromFile(filename string, calendarName string, color lipgloss.Color) ([]Event, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return loadICSFromReader(file, calendarName, color)
}

// Load calendars from Radicale server
func loadCalendarsFromRadicale(config *RadicaleConfig) ([]CalDAVCalendar, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Normalize server URL (remove trailing slash)
	serverURL := strings.TrimSuffix(config.ServerURL, "/")

	// Radicale typically uses /username/ as the user collection path
	// Try username-based path first, then root as fallback
	userPath := "/" + config.Username + "/"
	pathsToTry := []string{userPath, "/"}

	var calendars []CalDAVCalendar
	var lastErr error

	for _, basePath := range pathsToTry {
		// Discover calendars using PROPFIND
		fullURL := serverURL + basePath
		req, err := http.NewRequest("PROPFIND", fullURL, nil)
		if err != nil {
			lastErr = err
			continue
		}

		// Set authentication
		auth := base64.StdEncoding.EncodeToString([]byte(config.Username + ":" + config.Password))
		req.Header.Set("Authorization", "Basic "+auth)
		req.Header.Set("Content-Type", "application/xml")
		req.Header.Set("Depth", "1")

		// Create PROPFIND request body
		propfind := propfindRequest{
			Prop: prop{
				DisplayName: "",
			},
		}

		var buf bytes.Buffer
		buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
		enc := xml.NewEncoder(&buf)
		enc.Indent("", "  ")
		if err := enc.Encode(propfind); err != nil {
			lastErr = err
			continue
		}

		req.Body = io.NopCloser(&buf)
		req.ContentLength = int64(buf.Len())

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != 207 { // Multi-Status
			body, _ := io.ReadAll(resp.Body)
			bodyStr := string(body)
			if len(bodyStr) > 500 {
				bodyStr = bodyStr[:500] + "..."
			}
			lastErr = fmt.Errorf("failed to discover calendars at %s (status %d): %s", fullURL, resp.StatusCode, bodyStr)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = err
			continue
		}

		var ms multistatus
		if err := xml.Unmarshal(body, &ms); err != nil {
			lastErr = err
			continue
		}

		// If no responses, try next path
		if len(ms.Response) == 0 {
			continue
		}

		// Parse responses
		for _, r := range ms.Response {
			// Find the successful propstat (status 200)
			var successfulPropstat *propstat
			for i := range r.Propstat {
				if strings.Contains(r.Propstat[i].Status, "200") {
					successfulPropstat = &r.Propstat[i]
					break
				}
			}

			// Skip if no successful propstat found
			if successfulPropstat == nil {
				continue
			}

			// Filter out the collection itself and only get calendar collections
			href := r.Href
			// Normalize the href - handle relative and absolute paths
			if !strings.HasPrefix(href, "/") {
				// Relative path - prepend base path
				if !strings.HasSuffix(basePath, "/") {
					href = basePath + "/" + href
				} else {
					href = basePath + href
				}
			}
			// Ensure href ends with / for collections
			if !strings.HasSuffix(href, "/") {
				href += "/"
			}

			// Skip the base path itself
			normalizedBasePath := basePath
			if !strings.HasSuffix(normalizedBasePath, "/") {
				normalizedBasePath += "/"
			}
			if href == normalizedBasePath || href == "/" || href == "//" {
				continue
			}

			// Get calendar name from DisplayName property, fallback to path if not available
			calName := successfulPropstat.Prop.DisplayName
			if calName == "" {
				// Fallback to path-based name
				calName = path.Base(strings.TrimSuffix(href, "/"))
			}

			// Get path name for filtering
			pathName := path.Base(strings.TrimSuffix(href, "/"))

			// Skip system collections, but allow calendars under username path
			// Calendars can be at /username/ or /username/calendarname/
			skip := false
			if pathName == "user" || pathName == "principals" {
				skip = true
			}
			// Only skip if the pathName equals username AND it's a direct child of root
			// (not if it's a calendar under the username)
			if pathName == config.Username && strings.Count(href, "/") <= 2 {
				// This is the username collection itself, not a calendar
				skip = true
			}

			if !skip {
				// Construct full URL (normalize to avoid double slashes)
				calURL := serverURL + href
				calendars = append(calendars, CalDAVCalendar{
					DisplayName: calName,
					URL:         calURL,
				})
			}
		}

		// If we found calendars from this path, return them immediately
		// Don't try the next path to avoid duplicates
		if len(calendars) > 0 {
			return calendars, nil
		}
	}

	// If we got here, we didn't find any calendars
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no calendars found")
}

// Load events from a Radicale calendar
func loadICSFromRadicale(calendarURL string, calendarName string, color lipgloss.Color, config *RadicaleConfig) ([]Event, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Radicale calendars can be accessed via .ics extension
	// Try multiple URL formats
	baseURL := strings.TrimSuffix(calendarURL, "/")
	urlsToTry := []string{
		baseURL + ".ics",     // Standard Radicale format
		calendarURL + ".ics", // With trailing slash
		baseURL,              // Without .ics
		calendarURL,          // Original URL
	}

	var lastErr error
	var lastStatus int
	var lastBody string

	for _, url := range urlsToTry {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			lastErr = err
			continue
		}

		auth := base64.StdEncoding.EncodeToString([]byte(config.Username + ":" + config.Password))
		req.Header.Set("Authorization", "Basic "+auth)
		req.Header.Set("Accept", "text/calendar")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		lastStatus = resp.StatusCode
		body, _ := io.ReadAll(resp.Body)
		lastBody = string(body)

		if resp.StatusCode == http.StatusOK {
			// Check if it's actually calendar data (starts with BEGIN:VCALENDAR)
			if strings.HasPrefix(strings.TrimSpace(lastBody), "BEGIN:VCALENDAR") {
				// Try to parse as calendar
				events, err := loadICSFromReader(bytes.NewReader(body), calendarName, color)
				if err == nil {
					return events, nil
				}
				lastErr = fmt.Errorf("failed to parse calendar data: %v", err)
			} else {
				lastErr = fmt.Errorf("response is not calendar data (status: %d)", resp.StatusCode)
			}
		} else if resp.StatusCode == 207 {
			// Multi-status response - try to extract calendar data from XML
			return parseCalendarFromMultistatus(lastBody, calendarName, color)
		} else {
			// Log the error but try next URL
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, lastBody[:min(200, len(lastBody))])
		}
	}

	// If all URLs failed, return detailed error
	return nil, fmt.Errorf("failed to load calendar '%s' from %s (tried %d URLs, last: %d - %v)",
		calendarName, calendarURL, len(urlsToTry), lastStatus, lastErr)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Parse calendar data from CalDAV multistatus XML response
func parseCalendarFromMultistatus(xmlBody string, calendarName string, color lipgloss.Color) ([]Event, error) {
	// Look for calendar-data elements in the XML
	// This is a simple regex-based approach - a proper XML parser would be better
	re := regexp.MustCompile(`<C:calendar-data[^>]*>([\s\S]*?)</C:calendar-data>`)
	matches := re.FindAllStringSubmatch(xmlBody, -1)

	if len(matches) == 0 {
		return nil, fmt.Errorf("no calendar-data found in multistatus response")
	}

	// Combine all calendar data blocks
	var combinedCalendar strings.Builder
	combinedCalendar.WriteString("BEGIN:VCALENDAR\nVERSION:2.0\n")

	for _, match := range matches {
		if len(match) > 1 {
			// Decode XML entities and extract calendar content
			calData := match[1]
			calData = strings.ReplaceAll(calData, "&lt;", "<")
			calData = strings.ReplaceAll(calData, "&gt;", ">")
			calData = strings.ReplaceAll(calData, "&amp;", "&")
			calData = strings.ReplaceAll(calData, "&quot;", "\"")
			calData = strings.ReplaceAll(calData, "&apos;", "'")
			combinedCalendar.WriteString(calData)
		}
	}

	combinedCalendar.WriteString("END:VCALENDAR\n")

	// Parse the combined calendar
	return loadICSFromReader(strings.NewReader(combinedCalendar.String()), calendarName, color)
}

// Create event on Radicale server
func createEventOnRadicale(calendarURL string, event *Event, config *RadicaleConfig) error {
	// Generate a unique UID for the event
	if event.UID == "" {
		event.UID = fmt.Sprintf("%s@mytuicalendar", time.Now().Format("20060102T150405Z"))
	}

	// Create ICS content
	icsContent := fmt.Sprintf(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//MyTuiCalendar//EN
BEGIN:VEVENT
UID:%s
DTSTART:%s
DTEND:%s
SUMMARY:%s
DESCRIPTION:%s
END:VEVENT
END:VCALENDAR
`, event.UID,
		event.Start.Format("20060102T150405Z"),
		event.End.Format("20060102T150405Z"),
		escapeICSValue(event.Summary),
		escapeICSValue(event.Description))

	client := &http.Client{Timeout: 10 * time.Second}
	eventURL := calendarURL + "/" + event.UID + ".ics"

	req, err := http.NewRequest("PUT", eventURL, bytes.NewBufferString(icsContent))
	if err != nil {
		return err
	}

	auth := base64.StdEncoding.EncodeToString([]byte(config.Username + ":" + config.Password))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "text/calendar; charset=utf-8")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 && resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create event: %s - %s", resp.Status, string(body))
	}

	return nil
}

func escapeICSValue(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, ",", "\\,")
	value = strings.ReplaceAll(value, ";", "\\;")
	value = strings.ReplaceAll(value, "\n", "\\n")
	return value
}

func loadAllCalendars(radicaleConfig *RadicaleConfig) ([]Event, map[string]lipgloss.Color, map[string]string, error) {
	var allEvents []Event
	calendars := make(map[string]lipgloss.Color)
	calendarURLs := make(map[string]string)
	colorIndex := 0
	loadedCalendars := make(map[string]bool)

	config, configErr := loadConfig()
	if configErr == nil && config != nil {
		// Use config's Radicale if available, otherwise use passed parameter
		if config.Radicale != nil {
			radicaleConfig = config.Radicale
		}

		// Load Radicale calendars if configured
		if radicaleConfig != nil && radicaleConfig.ServerURL != "" {
			radicaleCals, err := loadCalendarsFromRadicale(radicaleConfig)
			if err == nil {
				for _, cal := range radicaleCals {
					color := calendarColors[colorIndex%len(calendarColors)]
					calendars[cal.DisplayName] = color
					calendarURLs[cal.DisplayName] = cal.URL

					events, err := loadICSFromRadicale(cal.URL, cal.DisplayName, color, radicaleConfig)
					if err == nil {
						allEvents = append(allEvents, events...)
					} else {
						fmt.Fprintf(os.Stderr, "Warning: Failed to load Radicale calendar %s: %v\n", cal.DisplayName, err)
					}
					colorIndex++
				}
			} else {
				fmt.Fprintf(os.Stderr, "Warning: Failed to connect to Radicale server: %v\n", err)
			}
		}

		// Load other calendars
		for _, cal := range config.Calendars {
			// Skip if it's a Radicale calendar (already loaded above)
			if cal.Type == "radicale" {
				continue
			}

			color := calendarColors[colorIndex%len(calendarColors)]
			calendars[cal.Name] = color

			var events []Event
			var err error

			if cal.URL != "" {
				events, err = loadICSFromURL(cal.URL, cal.Name, color)
			} else if cal.File != "" {
				events, err = loadICSFromFile(cal.File, cal.Name, color)
				loadedCalendars[cal.File] = true
			}

			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to load calendar %s: %v\n", cal.Name, err)
				continue
			}

			allEvents = append(allEvents, events...)
			colorIndex++
		}

		// Load local .ics files (only if listed in local_calendars)
		if len(config.LocalCalendars) > 0 {
			// Determine base directory: try current directory first (dev mode), then config directory
			var baseDir string
			localConfig := "calendars.json"
			if _, err := os.Stat(localConfig); err == nil {
				// Dev mode: use current directory
				baseDir = "."
			} else {
				// Build mode: use config directory
				configDir, err := getConfigDir()
				if err != nil {
					baseDir = ""
				} else {
					baseDir = configDir
				}
			}

			if baseDir != "" {
				for _, localCal := range config.LocalCalendars {
					// Construct full path to .ics file
					icsFile := localCal
					if !strings.HasSuffix(icsFile, ".ics") {
						icsFile += ".ics"
					}
					icsPath := filepath.Join(baseDir, icsFile)

					// Check if file exists
					if _, err := os.Stat(icsPath); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: Local calendar file not found: %s\n", icsPath)
						continue
					}

					calendarName := strings.TrimSuffix(filepath.Base(icsFile), ".ics")
					color := calendarColors[colorIndex%len(calendarColors)]
					calendars[calendarName] = color

					events, err := loadICSFromFile(icsPath, calendarName, color)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Warning: Failed to load local calendar %s: %v\n", calendarName, err)
						continue
					}

					allEvents = append(allEvents, events...)
					colorIndex++
				}
			}
		}
	}

	if len(allEvents) == 0 {
		return nil, nil, nil, fmt.Errorf("no calendars found")
	}

	return allEvents, calendars, calendarURLs, nil
}

func getNextEvent(events []Event) *Event {
	now := time.Now()
	var upcoming []Event

	for _, event := range events {
		if event.Start.After(now) {
			upcoming = append(upcoming, event)
		}
	}

	if len(upcoming) == 0 {
		return nil
	}

	sort.Slice(upcoming, func(i, j int) bool {
		return upcoming[i].Start.Before(upcoming[j].Start)
	})

	return &upcoming[0]
}

func renderNextEvent(event *Event) string {
	if event == nil {
		return noEventsStyle.Render("No upcoming events")
	}

	var boxContent strings.Builder

	timeStr := fmt.Sprintf("%s - %s",
		event.Start.Format("Mon Jan 2, 15:04"),
		event.End.Format("15:04"),
	)

	timeUntil := time.Until(event.Start)
	timeUntilStr := ""
	if timeUntil < time.Hour {
		timeUntilStr = fmt.Sprintf(" (in %dm)", int(timeUntil.Minutes()))
	} else if timeUntil < 24*time.Hour {
		timeUntilStr = fmt.Sprintf(" (in %.1fh)", timeUntil.Hours())
	} else {
		timeUntilStr = fmt.Sprintf(" (in %dd)", int(timeUntil.Hours()/24))
	}

	timeLineStyle := timeStyle.Foreground(lipgloss.Color("241"))
	boxContent.WriteString(timeLineStyle.Render(timeStr+timeUntilStr) + "\n")

	titleStyle := lipgloss.NewStyle().
		Foreground(event.CalendarColor).
		Bold(true)
	boxContent.WriteString(titleStyle.Render("● " + event.Summary))

	if event.Description != "" && strings.TrimSpace(event.Description) != "" {
		descStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Italic(true).
			Width(56)

		desc := strings.TrimSpace(event.Description)
		if len(desc) > 150 {
			desc = desc[:150] + "..."
		}
		boxContent.WriteString("\n" + descStyle.Render(desc))
	}

	boxStyle := eventBoxStyle.
		BorderForeground(event.CalendarColor).
		Width(60)

	return "\n" + titleStyle.Foreground(lipgloss.Color("86")).Bold(true).Render("📅 Next Event") + "\n\n" + boxStyle.Render(boxContent.String())
}

// expandRecurringEvent expands a recurring event based on RRULE
type occurrence struct {
	Start time.Time
	End   time.Time
}

func expandRecurringEvent(start, end time.Time, rrule string, maxDate time.Time, now time.Time) []occurrence {
	var occurrences []occurrence
	duration := end.Sub(start)

	// Parse RRULE - basic support for common patterns
	// Format: FREQ=DAILY|WEEKLY|MONTHLY|YEARLY[;INTERVAL=n][;COUNT=n][;UNTIL=YYYYMMDDTHHMMSSZ]
	rrule = strings.ToUpper(rrule)
	
	var freq string
	interval := 1
	var until time.Time
	count := -1

	parts := strings.Split(rrule, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "FREQ=") {
			freq = strings.TrimPrefix(part, "FREQ=")
		} else if strings.HasPrefix(part, "INTERVAL=") {
			if val, err := strconv.Atoi(strings.TrimPrefix(part, "INTERVAL=")); err == nil {
				interval = val
			}
		} else if strings.HasPrefix(part, "UNTIL=") {
			untilStr := strings.TrimPrefix(part, "UNTIL=")
			// Try parsing different date formats
			if t, err := time.Parse("20060102T150405Z", untilStr); err == nil {
				until = t
			} else if t, err := time.Parse("20060102T150405", untilStr); err == nil {
				until = t
			} else if t, err := time.Parse("20060102", untilStr); err == nil {
				until = t
			}
		} else if strings.HasPrefix(part, "COUNT=") {
			if val, err := strconv.Atoi(strings.TrimPrefix(part, "COUNT=")); err == nil {
				count = val
			}
		}
	}

	// Determine end date
	endDate := maxDate
	if !until.IsZero() && until.Before(maxDate) {
		endDate = until
	}

	// Start from the original start date
	currentStart := start
	iteration := 0
	maxIterations := 1000 // Safety limit

	// Check if we need to fast-forward past occurrences
	// Only fast-forward if the event is more than 1 day in the past
	// We want to include events from yesterday (they're still relevant)
	originalIsToday := currentStart.Format("2006-01-02") == now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1)
	originalIsYesterday := currentStart.Format("2006-01-02") == yesterday.Format("2006-01-02")
	// Only fast-forward if it's before yesterday (more than 1 day old)
	needsFastForward := currentStart.Before(yesterday) && !originalIsToday && !originalIsYesterday
	
	// If the original event is today or in the future, we'll include it in the loop
	// If it's in the past (not today), we need to fast-forward to today or the next occurrence
	if needsFastForward {
		// For past events, fast-forward to today's occurrence (if it exists) or the next occurrence after now
		// We want to include today's occurrence even if the event started in the past
		todayDate := now.Format("2006-01-02")
		switch freq {
		case "DAILY":
			// Fast-forward until we reach today (date-wise) or the future
			for {
				nextStart := currentStart.AddDate(0, 0, interval)
				nextDate := nextStart.Format("2006-01-02")
				
				// Stop if we've reached today (same date) - regardless of time
				// OR if we've reached the future
				if nextDate == todayDate {
					currentStart = nextStart
					break
				}
				
				// If we've reached the future (after today), stop
				if nextStart.After(now) {
					currentStart = nextStart
					break
				}
				
				// If still in the past (before today), continue
				currentStart = nextStart
			}
		case "WEEKLY":
			// Fast-forward until we reach today (date-wise) or the future
			for {
				nextStart := currentStart.AddDate(0, 0, 7*interval)
				nextDate := nextStart.Format("2006-01-02")
				if nextDate == todayDate {
					currentStart = nextStart
					break
				}
				if nextStart.After(now) {
					currentStart = nextStart
					break
				}
				currentStart = nextStart
			}
		case "MONTHLY":
			// Fast-forward until we reach today (date-wise) or the future
			for {
				nextStart := currentStart.AddDate(0, interval, 0)
				nextDate := nextStart.Format("2006-01-02")
				if nextDate == todayDate {
					currentStart = nextStart
					break
				}
				if nextStart.After(now) {
					currentStart = nextStart
					break
				}
				currentStart = nextStart
			}
		case "YEARLY":
			// Fast-forward until we reach today (date-wise) or the future
			for {
				nextStart := currentStart.AddDate(interval, 0, 0)
				nextDate := nextStart.Format("2006-01-02")
				if nextDate == todayDate {
					currentStart = nextStart
					break
				}
				if nextStart.After(now) {
					currentStart = nextStart
					break
				}
				currentStart = nextStart
			}
		default:
			// Unknown frequency, return empty
			return occurrences
		}
		// Make sure we don't skip too far
		if currentStart.After(endDate) {
			return occurrences
		}
	} else {
		// Original event is today or in the future - start from the original start
		// This ensures we include the first occurrence
		currentStart = start
	}

	// Generate occurrences starting from currentStart
	// Always include the first occurrence if it's today or in the future
	for currentStart.Before(endDate) && iteration < maxIterations {
		if count > 0 && iteration >= count {
			break
		}

		// Include occurrences that are yesterday, today, or in the future
		// We include yesterday's events because they're still relevant (just happened)
		occIsToday := currentStart.Format("2006-01-02") == now.Format("2006-01-02")
		occIsYesterday := currentStart.Format("2006-01-02") == yesterday.Format("2006-01-02")
		occIsFuture := currentStart.After(now)
		
		// Always include if it's yesterday, today, or in the future
		if occIsYesterday || occIsToday || occIsFuture {
			occurrences = append(occurrences, occurrence{
				Start: currentStart,
				End:   currentStart.Add(duration),
			})
		}

		// Move to next occurrence based on frequency
		switch freq {
		case "DAILY":
			currentStart = currentStart.AddDate(0, 0, interval)
		case "WEEKLY":
			currentStart = currentStart.AddDate(0, 0, 7*interval)
		case "MONTHLY":
			currentStart = currentStart.AddDate(0, interval, 0)
		case "YEARLY":
			currentStart = currentStart.AddDate(interval, 0, 0)
		default:
			// Unknown frequency, stop expansion
			return occurrences
		}

		iteration++
	}

	return occurrences
}
