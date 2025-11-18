package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m model) viewNaturalLanguage() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("📝 Create Event (Natural Language)") + "\n")
	b.WriteString(helpStyle.Render("Example: 'Meeting tomorrow at 3pm for 1 hour'") + "\n\n")
	b.WriteString(inputStyle.Render("Input: ") + m.naturalLangInput + "▊\n\n")

	if m.naturalLangInput != "" {
		event, err := parseNaturalLanguage(m.naturalLangInput, m.currentDate)
		if err == nil {
			preview := fmt.Sprintf("Summary: %s\nStart: %s\nEnd: %s\nCalendar: %s",
				event.Summary,
				event.Start.Format("Mon Jan 2, 2006 15:04"),
				event.End.Format("15:04"),
				m.selectedCalendar)
			b.WriteString(eventBoxStyle.Width(60).Render(preview) + "\n")
		} else {
			b.WriteString(helpStyle.Render(fmt.Sprintf("Parse error: %v", err)) + "\n")
		}
	}

	b.WriteString("\n" + helpStyle.Render("Enter: confirm | Esc: cancel | l: switch to form mode | Calendar: "+m.selectedCalendar))
	if m.message != "" {
		b.WriteString("\n" + helpStyle.Render(m.message))
	}

	return b.String()
}

func (m model) viewLoading() string {
	progressView := m.loadingProgress.View()

	var b strings.Builder
	b.WriteString(titleStyle.Render("Loading Calendars...") + "\n\n")
	if m.loadingMessage != "" {
		b.WriteString(helpStyle.Render(m.loadingMessage) + "\n")
	}
	b.WriteString(progressView + "\n")

	return b.String()
}

func (m model) viewEventForm() string {
	// Set form width to leave room for summary
	formWidth := 50
	summaryWidth := 30
	if m.width > 0 {
		formWidth = (m.width * 60) / 100
		summaryWidth = m.width - formWidth - 4
		if summaryWidth < 25 {
			summaryWidth = 25
		}
		if formWidth < 40 {
			formWidth = 40
		}
		// Update form width (WithWidth returns a new form, but we'll handle this in Update)
		// Don't modify form in View - it's already set in Update via WindowSizeMsg
	}

	formView := m.eventForm.View()

	// Create summary box
	summaryBox := m.renderFormSummary()

	// Create two-column layout
	leftColumn := lipgloss.NewStyle().Width(formWidth).Render(formView)
	rightColumn := lipgloss.NewStyle().Width(summaryWidth).Render(summaryBox)

	content := lipgloss.JoinHorizontal(lipgloss.Top, leftColumn, "  ", rightColumn)

	// Add help bar at the bottom
	helpText := "Enter: confirm & next | Shift+Tab: previous | Esc: cancel"
	helpBar := helpStyle.Render(helpText)

	// Calculate available height for content (leave room for help bar)
	availableHeight := m.height - 1
	if availableHeight < 1 {
		availableHeight = 1
	}

	// Split content into lines for scrolling
	contentLines := strings.Split(content, "\n")
	totalLines := len(contentLines)

	// Adjust scroll offset if needed
	if totalLines > availableHeight {
		// Ensure scroll offset is within bounds
		maxOffset := totalLines - availableHeight
		if m.formScrollOffset > maxOffset {
			m.formScrollOffset = maxOffset
		}
		if m.formScrollOffset < 0 {
			m.formScrollOffset = 0
		}

		// Get visible lines
		start := m.formScrollOffset
		end := start + availableHeight
		if end > totalLines {
			end = totalLines
		}
		visibleLines := contentLines[start:end]
		content = strings.Join(visibleLines, "\n")
	} else {
		// Content fits, reset scroll
		m.formScrollOffset = 0
	}

	// Combine content and help bar
	return lipgloss.JoinVertical(lipgloss.Left, content, helpBar)
}
func (m model) viewDaily() string {
	var b strings.Builder

	title := titleStyle.Render("📅 Daily View")
	b.WriteString(title + "\n")

	_, week := m.currentDate.ISOWeek()
	dateHeader := dateHeaderStyle.Render(fmt.Sprintf(
		"%s, %s (Week %d)",
		m.currentDate.Format("Monday"),
		m.currentDate.Format("January 2, 2006"),
		week,
	))
	b.WriteString(dateHeader + "\n")

	dayEvents := m.getEventsForDay(m.currentDate)
	currentTime := time.Now()

	if len(dayEvents) == 0 {
		b.WriteString(noEventsStyle.Render("No events scheduled for this day") + "\n")
	} else {
		boxWidth := 60
		if m.width > 0 {
			boxWidth = m.width - 10
			if boxWidth > 80 {
				boxWidth = 80
			}
			if boxWidth < 40 {
				boxWidth = 40
			}
		}

		for _, event := range dayEvents {
			isNow := m.currentDate.Format("2006-01-02") == currentTime.Format("2006-01-02") &&
				currentTime.After(event.Start) && currentTime.Before(event.End)

			var boxContent strings.Builder

			timeStr := fmt.Sprintf("%s - %s",
				event.Start.Format("15:04"),
				event.End.Format("15:04"),
			)
			duration := event.End.Sub(event.Start)
			durationStr := ""
			if duration >= time.Hour {
				durationStr = fmt.Sprintf(" (%.1fh)", duration.Hours())
			} else if duration > 0 {
				durationStr = fmt.Sprintf(" (%dm)", int(duration.Minutes()))
			}

			timeLineStyle := timeStyle.Foreground(lipgloss.Color("241"))
			boxContent.WriteString(timeLineStyle.Render(timeStr+durationStr) + "\n")

			titleStyle := lipgloss.NewStyle().
				Foreground(event.CalendarColor).
				Bold(true)
			boxContent.WriteString(titleStyle.Render("● " + event.Summary))

			if event.Description != "" && strings.TrimSpace(event.Description) != "" {
				descStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("245")).
					Italic(true).
					Width(boxWidth - 4)

				desc := strings.TrimSpace(event.Description)
				if len(desc) > 150 {
					desc = desc[:150] + "..."
				}
				boxContent.WriteString("\n" + descStyle.Render(desc))
			}

			boxStyle := eventBoxStyle.
				BorderForeground(event.CalendarColor).
				Width(boxWidth)

			if isNow {
				boxStyle = boxStyle.
					BorderForeground(lipgloss.Color("205")).
					BorderStyle(lipgloss.ThickBorder())
			}

			b.WriteString(boxStyle.Render(boxContent.String()) + "\n")
		}
	}

	if !m.oneShot {
		b.WriteString(m.renderCalendarLegend())
		b.WriteString("\n" + helpStyle.Render("d: daily  w: weekly  m: monthly  |  ← →: navigate  t: today  |  n: new event  |  q: quit"))

		if m.err != nil {
			b.WriteString("\n" + helpStyle.Render("Note: Using sample data (no calendars found)"))
		}
	}

	return b.String()
}

func (m model) viewWeekly() string {
	var b strings.Builder

	title := titleStyle.Render("📅 Weekly View")
	b.WriteString(title + "\n")

	weekStart := m.getWeekStart(m.currentDate)
	_, week := weekStart.ISOWeek()

	dateHeader := dateHeaderStyle.Render(fmt.Sprintf(
		"Week %d - %s to %s",
		week,
		weekStart.Format("Jan 2"),
		weekStart.AddDate(0, 0, 6).Format("Jan 2, 2006"),
	))
	b.WriteString(dateHeader + "\n")

	for i := 0; i < 7; i++ {
		day := weekStart.AddDate(0, 0, i)
		dayEvents := m.getEventsForDay(day)

		dayHeader := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("117")).
			Render(day.Format("Monday, Jan 2"))

		b.WriteString("\n" + dayHeader + "\n")

		if len(dayEvents) == 0 {
			b.WriteString(noEventsStyle.Render("  No events") + "\n")
		} else {
			for _, event := range dayEvents {
				timeStr := fmt.Sprintf("  %s - %s",
					event.Start.Format("15:04"),
					event.End.Format("15:04"),
				)
				b.WriteString(timeStyle.Render(timeStr))

				eventStyle := lipgloss.NewStyle().
					Foreground(event.CalendarColor).
					MarginLeft(2)

				b.WriteString(eventStyle.Render(fmt.Sprintf("● %s", event.Summary)))
				b.WriteString("\n")
			}
		}
	}

	if !m.oneShot {
		b.WriteString(m.renderCalendarLegend())
		b.WriteString("\n" + helpStyle.Render("d: daily  w: weekly  m: monthly  |  ← →: navigate  t: today  |  n: new event  |  q: quit"))
	}

	return b.String()
}

func (m model) viewMonthly() string {
	var b strings.Builder

	title := titleStyle.Render("📅 Monthly View")
	b.WriteString(title + "\n")

	dateHeader := dateHeaderStyle.Render(m.currentDate.Format("January 2006"))
	b.WriteString(dateHeader + "\n")

	weekdays := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	var headerRow strings.Builder
	for _, day := range weekdays {
		headerRow.WriteString(weekdayHeaderStyle.Render(day))
	}
	b.WriteString(headerRow.String() + "\n")

	firstDay := time.Date(m.currentDate.Year(), m.currentDate.Month(), 1, 0, 0, 0, 0, time.Local)
	lastDay := time.Date(m.currentDate.Year(), m.currentDate.Month()+1, 0, 0, 0, 0, 0, time.Local)

	startWeekday := int(firstDay.Weekday())
	if startWeekday == 0 {
		startWeekday = 7
	}
	startWeekday--

	day := 1
	today := time.Now()

	for week := 0; week < 6; week++ {
		var row []string
		for weekday := 0; weekday < 7; weekday++ {
			if (week == 0 && weekday < startWeekday) || day > lastDay.Day() {
				row = append(row, cellStyle.Render(""))
			} else {
				cellDate := time.Date(m.currentDate.Year(), m.currentDate.Month(), day, 0, 0, 0, 0, time.Local)
				cell := m.renderMonthCell(cellDate, today)
				row = append(row, cell)
				day++
			}
		}
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, row...) + "\n")

		if day > lastDay.Day() {
			break
		}
	}

	if !m.oneShot {
		b.WriteString(m.renderCalendarLegend())
		if m.dayInput != "" {
			b.WriteString("\n" + helpStyle.Render(fmt.Sprintf("Jump to day: %s (press Enter)", m.dayInput)))
		}
		b.WriteString("\n" + helpStyle.Render("d: daily  w: weekly  m: monthly  |  ← →: navigate  t: today  |  0-9 + Enter: jump  |  n: new event  |  q: quit"))
	}

	return b.String()
}

func (m model) renderMonthCell(date time.Time, today time.Time) string {
	var content strings.Builder

	isToday := date.Format("2006-01-02") == today.Format("2006-01-02")
	dayStyle := lipgloss.NewStyle().Bold(true)
	if isToday {
		dayStyle = dayStyle.Foreground(lipgloss.Color("205"))
	}
	content.WriteString(dayStyle.Render(fmt.Sprintf("%2d", date.Day())) + "\n")

	durationPerCalendar := make(map[string]time.Duration)
	hasEventsPerCalendar := make(map[string]bool)
	dayEvents := m.getEventsForDay(date)

	for _, event := range dayEvents {
		duration := event.End.Sub(event.Start)
		durationPerCalendar[event.CalendarName] += duration
		hasEventsPerCalendar[event.CalendarName] = true
	}

	if len(hasEventsPerCalendar) > 0 {
		var calNames []string
		for name := range m.calendars {
			if hasEventsPerCalendar[name] {
				calNames = append(calNames, name)
			}
		}
		sort.Strings(calNames)

		maxHeight := 2
		barHeights := make([]int, len(calNames))
		colors := make([]lipgloss.Color, len(calNames))

		for i, calName := range calNames {
			duration := durationPerCalendar[calName]
			colors[i] = m.calendars[calName]

			hours := duration.Hours()
			barHeight := int(hours / 2)
			if barHeight > maxHeight {
				barHeight = maxHeight
			}
			if barHeight < 1 {
				barHeight = 1
			}
			barHeights[i] = barHeight
		}

		for row := maxHeight; row >= 1; row-- {
			content.WriteString("\n")
			for i := 0; i < len(barHeights); i++ {
				if barHeights[i] >= row {
					barStyle := lipgloss.NewStyle().Foreground(colors[i])
					content.WriteString(barStyle.Render("█"))
				} else {
					content.WriteString(" ")
				}
			}
		}
	}

	style := cellStyle
	if isToday {
		style = todayCellStyle
	}

	return style.Render(content.String())
}

func (m model) renderCalendarLegend() string {
	var b strings.Builder
	b.WriteString(calendarLabelStyle.Render("Calendars:") + "\n")
	for name, color := range m.calendars {
		legendStyle := lipgloss.NewStyle().
			Foreground(color).
			Padding(0, 1)
		b.WriteString(legendStyle.Render(fmt.Sprintf("● %s", name)))
	}
	return b.String()
}

func (m model) getEventsForDay(date time.Time) []Event {
	var dayEvents []Event
	for _, event := range m.events {
		if event.Start.Year() == date.Year() &&
			event.Start.Month() == date.Month() &&
			event.Start.Day() == date.Day() {
			dayEvents = append(dayEvents, event)
		}
	}

	sort.Slice(dayEvents, func(i, j int) bool {
		return dayEvents[i].Start.Before(dayEvents[j].Start)
	})

	return dayEvents
}

func (m model) getWeekStart(date time.Time) time.Time {
	weekday := int(date.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return date.AddDate(0, 0, -(weekday - 1))
}
