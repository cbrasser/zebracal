package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// buildEventForm creates a huh form for event creation
func buildEventForm(summary, description, dateStr, startTime, endTime, selectedCal *string, repeatOption *string, repeatEndDate *string, calendars map[string]lipgloss.Color) *huh.Form {
	// Build calendar options
	calOptions := make([]huh.Option[string], 0, len(calendars))
	calNames := make([]string, 0, len(calendars))
	for name := range calendars {
		calNames = append(calNames, name)
	}
	sort.Strings(calNames)
	for _, name := range calNames {
		calOptions = append(calOptions, huh.NewOption(name, name))
	}

	// Check if a repeat option is selected (excluding "none")
	hasRepeat := func() bool {
		return repeatOption != nil && *repeatOption != "" && *repeatOption != "none"
	}

	// Build base fields
	fields := []huh.Field{
		huh.NewInput().
			Title("Event Summary").
			Prompt("> ").
			Value(summary).
			Placeholder("Meeting with team").
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("summary cannot be empty")
				}
				return nil
			}),

		huh.NewText().
			Title("Description").
			Value(description).
			Placeholder("Optional description"),

		huh.NewInput().
			Title("Date").
			Prompt("> ").
			Value(dateStr).
			Placeholder("DD-MM-YYYY").
			Validate(func(s string) error {
				_, err := time.Parse("02-01-2006", s)
				return err
			}),

		huh.NewInput().
			Title("Start Time").
			Prompt("> ").
			Value(startTime).
			Placeholder("HH:MM").
			Validate(func(s string) error {
				if s == "" {
					return nil // Optional field
				}
				_, err := time.Parse("15:04", s)
				return err
			}),

		huh.NewInput().
			Title("End Time").
			Prompt("> ").
			Value(endTime).
			Placeholder("HH:MM").
			Validate(func(s string) error {
				if s == "" {
					return nil // Optional field
				}
				_, err := time.Parse("15:04", s)
				return err
			}),

		huh.NewSelect[string]().
			Title("Calendar").
			Options(calOptions...).
			Value(selectedCal),

		huh.NewSelect[string]().
			Title("Repetition").
			Options(
				huh.NewOption("None", "none"),
				huh.NewOption("Daily", "daily"),
				huh.NewOption("Weekly", "weekly"),
				huh.NewOption("Monthly", "monthly"),
			).
			Value(repeatOption),
	}

	// Only add "Repeat Until" field if a repeat option (other than "none") is selected
	if hasRepeat() {
		fields = append(fields, huh.NewInput().
			Title("Repeat Until (DD-MM-YYYY)").
			Prompt("> ").
			Value(repeatEndDate).
			Placeholder("DD-MM-YYYY (optional)").
			Validate(func(s string) error {
				if s == "" {
					return nil // Optional field
				}
				_, err := time.Parse("02-01-2006", s)
				return err
			}))
	}

	return huh.NewForm(
		huh.NewGroup(fields...),
	).WithTheme(huh.ThemeCharm())
}

func (m model) saveEventFromForm() (tea.Model, tea.Cmd) {
	// Parse form data - DD-MM-YYYY format
	date, err := time.Parse("02-01-2006", *m.formDate)
	if err != nil {
		m.message = fmt.Sprintf("Invalid date: %v (use DD-MM-YYYY)", err)
		m.creationMode = NoCreation
		m.eventForm = buildEventForm(m.formSummary, m.formDescription, m.formDate, m.formStartTime, m.formEndTime, m.formCalendar, m.formRepeatOptions, m.formRepeatEndDate, m.calendars)
		return m, m.eventForm.Init()
	}

	// Parse times (optional - can be empty)
	var start, end time.Time
	if *m.formStartTime != "" && *m.formEndTime != "" {
		startTime, err1 := time.Parse("15:04", *m.formStartTime)
		endTime, err2 := time.Parse("15:04", *m.formEndTime)
		if err1 != nil || err2 != nil {
			m.message = "Invalid time format (use HH:MM)"
			m.creationMode = NoCreation
			m.eventForm = buildEventForm(m.formSummary, m.formDescription, m.formDate, m.formStartTime, m.formEndTime, m.formCalendar, m.formRepeatOptions, m.formRepeatEndDate, m.calendars)
			return m, m.eventForm.Init()
		}

		start = time.Date(date.Year(), date.Month(), date.Day(),
			startTime.Hour(), startTime.Minute(), 0, 0, date.Location())
		end = time.Date(date.Year(), date.Month(), date.Day(),
			endTime.Hour(), endTime.Minute(), 0, 0, date.Location())
	} else {
		// If times are empty, use start of day and end of day
		start = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
		end = time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 0, 0, date.Location())
	}

	// Only validate time order if both times are provided
	if *m.formStartTime != "" && *m.formEndTime != "" {
		if end.Before(start) || end.Equal(start) {
			m.message = "End time must be after start time"
			m.creationMode = NoCreation
			m.eventForm = buildEventForm(m.formSummary, m.formDescription, m.formDate, m.formStartTime, m.formEndTime, m.formCalendar, m.formRepeatOptions, m.formRepeatEndDate, m.calendars)
			return m, m.eventForm.Init()
		}
	}

	// Determine repeat interval from single select
	repeatType := ""
	if m.formRepeatOptions != nil && *m.formRepeatOptions != "" && *m.formRepeatOptions != "none" {
		repeatType = *m.formRepeatOptions
	}

	// Parse repeat end date if provided - DD-MM-YYYY format
	var repeatEnd time.Time
	if repeatType != "" && m.formRepeatEndDate != nil && *m.formRepeatEndDate != "" {
		repeatEnd, err = time.Parse("02-01-2006", *m.formRepeatEndDate)
		if err != nil {
			m.message = fmt.Sprintf("Invalid repeat end date: %v (use DD-MM-YYYY)", err)
			m.creationMode = NoCreation
			m.eventForm = buildEventForm(m.formSummary, m.formDescription, m.formDate, m.formStartTime, m.formEndTime, m.formCalendar, m.formRepeatOptions, m.formRepeatEndDate, m.calendars)
			return m, m.eventForm.Init()
		}
	}

	// Create events (single or recurring)
	var eventsToCreate []*Event

	if repeatType != "" {
		// Create recurring events for the selected repeat type
		currentStart := start
		currentEnd := end
		maxIterations := 365 // Safety limit
		iteration := 0

		for iteration < maxIterations {
			event := &Event{
				Summary:      *m.formSummary,
				Description:  *m.formDescription,
				Start:        currentStart,
				End:          currentEnd,
				CalendarName: *m.formCalendar,
			}

			if color, ok := m.calendars[*m.formCalendar]; ok {
				event.CalendarColor = color
			}

			eventsToCreate = append(eventsToCreate, event)

			// Check if we've reached the end date
			if !repeatEnd.IsZero() && currentStart.After(repeatEnd) {
				break
			}

			// Move to next occurrence based on repeat type
			switch repeatType {
			case "daily":
				currentStart = currentStart.AddDate(0, 0, 1)
				currentEnd = currentEnd.AddDate(0, 0, 1)
			case "weekly":
				currentStart = currentStart.AddDate(0, 0, 7)
				currentEnd = currentEnd.AddDate(0, 0, 7)
			case "monthly":
				currentStart = currentStart.AddDate(0, 1, 0)
				currentEnd = currentEnd.AddDate(0, 1, 0)
			}

			// If no end date specified, create a reasonable number of occurrences
			if repeatEnd.IsZero() && iteration >= 52 { // Stop after 52 weeks for weekly, etc.
				break
			}

			iteration++
		}
	} else {
		// Single event
		event := &Event{
			Summary:      *m.formSummary,
			Description:  *m.formDescription,
			Start:        start,
			End:          end,
			CalendarName: *m.formCalendar,
		}

		if color, ok := m.calendars[*m.formCalendar]; ok {
			event.CalendarColor = color
		}

		eventsToCreate = append(eventsToCreate, event)
	}

	// Save events to Radicale if configured, otherwise save locally
	savedCount := 0
	for _, event := range eventsToCreate {
		if m.radicaleConfig != nil && m.calendarURLs[*m.formCalendar] != "" {
			if err := createEventOnRadicale(m.calendarURLs[*m.formCalendar], event, m.radicaleConfig); err != nil {
				m.message = fmt.Sprintf("Error creating event: %v", err)
				m.creationMode = NoCreation
				m.eventForm = buildEventForm(m.formSummary, m.formDescription, m.formDate, m.formStartTime, m.formEndTime, m.formCalendar, m.formRepeatOptions, m.formRepeatEndDate, m.calendars)
				return m, m.eventForm.Init()
			}
		}
		m.events = append(m.events, *event)
		savedCount++
	}

	if savedCount > 0 {
		if savedCount == 1 {
			m.message = "Event created successfully!"
		} else {
			m.message = fmt.Sprintf("%d events created successfully!", savedCount)
		}
	}

	m.creationMode = NoCreation
	// Rebuild form for next time
	m.eventForm = buildEventForm(m.formSummary, m.formDescription, m.formDate, m.formStartTime, m.formEndTime, m.formCalendar, m.formRepeatOptions, m.formRepeatEndDate, m.calendars)
	return m, m.eventForm.Init()
}

func (m model) renderFormSummary() string {
	var b strings.Builder

	summaryStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("117")).
		Padding(1, 2).
		Width(30)

	b.WriteString(titleStyle.Render("Event Summary") + "\n\n")

	if m.formSummary != nil && *m.formSummary != "" {
		b.WriteString(fmt.Sprintf("Summary: %s\n", *m.formSummary))
	} else {
		b.WriteString("Summary: (not set)\n")
	}

	if m.formDescription != nil && *m.formDescription != "" {
		desc := *m.formDescription
		if len(desc) > 40 {
			desc = desc[:40] + "..."
		}
		b.WriteString(fmt.Sprintf("Description: %s\n", desc))
	}

	if m.formDate != nil && *m.formDate != "" {
		b.WriteString(fmt.Sprintf("Date: %s\n", *m.formDate))
	}

	if m.formStartTime != nil && m.formEndTime != nil {
		b.WriteString(fmt.Sprintf("Time: %s - %s\n", *m.formStartTime, *m.formEndTime))
	}

	if m.formCalendar != nil && *m.formCalendar != "" {
		b.WriteString(fmt.Sprintf("Calendar: %s\n", *m.formCalendar))
	}

	if m.formRepeatOptions != nil && *m.formRepeatOptions != "" && *m.formRepeatOptions != "none" {
		// Capitalize first letter for display
		opt := *m.formRepeatOptions
		displayOpt := opt
		if len(opt) > 0 {
			displayOpt = strings.ToUpper(opt[:1]) + opt[1:]
		}
		b.WriteString(fmt.Sprintf("Repeat: %s\n", displayOpt))
		if m.formRepeatEndDate != nil && *m.formRepeatEndDate != "" {
			b.WriteString(fmt.Sprintf("Until: %s\n", *m.formRepeatEndDate))
		}
	}

	return summaryStyle.Render(b.String())
}
