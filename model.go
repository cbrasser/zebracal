package main

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

func initialModel(viewMode ViewMode, oneShot bool, radicaleConfig *RadicaleConfig) model {
	currentDate := time.Now()

	events, calendars, calendarURLs, err := loadAllCalendars(radicaleConfig)
	if err != nil {
		events = []Event{
			{
				Summary:       "Team Standup",
				Start:         time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 9, 0, 0, 0, time.Local),
				End:           time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 9, 30, 0, 0, time.Local),
				CalendarName:  "Work",
				CalendarColor: calendarColors[0],
			},
			{
				Summary:       "Lunch Break",
				Start:         time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 12, 0, 0, 0, time.Local),
				End:           time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 13, 0, 0, 0, time.Local),
				CalendarName:  "Personal",
				CalendarColor: calendarColors[1],
			},
		}
		calendars = map[string]lipgloss.Color{
			"Work":     calendarColors[0],
			"Personal": calendarColors[1],
		}
		calendarURLs = make(map[string]string)
	}

	// Set default selected calendar
	var defaultCalendar string
	for name := range calendars {
		defaultCalendar = name
		break
	}

	// Initialize progress bar
	prog := progress.New(progress.WithScaledGradient("#FF7CCB", "#FDFF8C"))
	prog.Width = 40

	// Initialize form data
	summary := ""
	description := ""
	dateStr := currentDate.Format("02-01-2006") // DD-MM-YYYY format
	startTime := "09:00"
	endTime := "10:00"
	selectedCal := defaultCalendar
	repeatOptions := "none"
	repeatEndDate := ""

	// Build event form
	eventForm := buildEventForm(&summary, &description, &dateStr, &startTime, &endTime, &selectedCal, &repeatOptions, &repeatEndDate, calendars)

	return model{
		events:           events,
		calendars:        calendars,
		calendarURLs:     calendarURLs,
		currentDate:      currentDate,
		viewMode:         viewMode,
		oneShot:          oneShot,
		err:              err,
		radicaleConfig:   radicaleConfig,
		selectedCalendar: defaultCalendar,
		uiFormState: UIFormState{
			date:      currentDate,
			startTime: "09:00",
			endTime:   "10:00",
		},
		eventForm:         eventForm,
		loadingProgress:   prog,
		isLoading:         false,
		formSummary:       &summary,
		formDescription:   &description,
		formDate:          &dateStr,
		formStartTime:     &startTime,
		formEndTime:       &endTime,
		formCalendar:      &selectedCal,
		formRepeatOptions: &repeatOptions,
		formRepeatEndDate: &repeatEndDate,
		formScrollOffset:  0,
	}
}
func (m model) Init() tea.Cmd {
	if m.oneShot {
		return tea.Quit
	}
	if m.eventForm != nil {
		return m.eventForm.Init()
	}
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// If we're in form mode, handle ALL messages through the form first
	// This gives the form complete control over its own state
	if m.creationMode == UIFormInput && m.eventForm != nil {
		// Handle window size for form
		if wmsg, ok := msg.(tea.WindowSizeMsg); ok {
			m.width = wmsg.Width
			m.height = wmsg.Height
			m.loadingProgress.Width = m.width - 10
			// Update form width
			m.eventForm = m.eventForm.WithWidth(m.width)
			// Also pass to form
			form, cmd := m.eventForm.Update(msg)
			if f, ok := form.(*huh.Form); ok {
				m.eventForm = f
			}
			return m, cmd
		}

		// Pass ALL messages directly to the form
		form, cmd := m.eventForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.eventForm = f
		}

		// Check form state after it processes the message
		if m.eventForm.State == huh.StateCompleted {
			return m.saveEventFromForm()
		}

		if m.eventForm.State == huh.StateAborted {
			m.creationMode = NoCreation
			m.formScrollOffset = 0
			m.message = ""
			// Rebuild form for next time
			m.eventForm = buildEventForm(m.formSummary, m.formDescription, m.formDate, m.formStartTime, m.formEndTime, m.formCalendar, m.formRepeatOptions, m.formRepeatEndDate, m.calendars)
			return m, m.eventForm.Init()
		}

		// Return form's command - critical for form to work properly
		return m, cmd
	}

	// Main view handling (only when NOT in form mode)
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.loadingProgress.Width = m.width - 10
		return m, nil

	case progress.FrameMsg:
		if m.isLoading {
			prog, cmd := m.loadingProgress.Update(msg)
			m.loadingProgress = prog.(progress.Model)
			return m, cmd
		}
		return m, nil

	case loadingMsg:
		m.isLoading = true
		m.loadingMessage = msg.message
		cmd := m.loadingProgress.SetPercent(msg.progress)
		return m, cmd

	case loadingCompleteMsg:
		m.isLoading = false
		m.loadingMessage = ""
		return m, nil

	case tea.KeyMsg:

		// Handle event creation mode (natural language)
		if m.creationMode == NaturalLanguageInput {
			// Allow switching back to form mode with 'l' key
			if msg.String() == "l" {
				m.creationMode = UIFormInput
				// Rebuild form
				m.eventForm = buildEventForm(m.formSummary, m.formDescription, m.formDate, m.formStartTime, m.formEndTime, m.formCalendar, m.formRepeatOptions, m.formRepeatEndDate, m.calendars)
				return m, m.eventForm.Init()
			}
			return m.handleEventCreationInput(msg)
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "n", "a": // 'n' for new, 'a' for add
			m.creationMode = UIFormInput
			// Reset form values
			*m.formSummary = ""
			*m.formDescription = ""
			*m.formDate = m.currentDate.Format("02-01-2006") // DD-MM-YYYY format
			*m.formStartTime = ""                            // No default
			*m.formEndTime = ""                              // No default
			*m.formCalendar = m.selectedCalendar
			*m.formRepeatOptions = "none" // Default to "None"
			*m.formRepeatEndDate = ""
			m.formScrollOffset = 0
			// Rebuild form
			m.eventForm = buildEventForm(m.formSummary, m.formDescription, m.formDate, m.formStartTime, m.formEndTime, m.formCalendar, m.formRepeatOptions, m.formRepeatEndDate, m.calendars)
			return m, m.eventForm.Init()
		case "left", "h":
			if m.viewMode == DailyView {
				m.currentDate = m.currentDate.AddDate(0, 0, -1)
			} else if m.viewMode == WeeklyView {
				m.currentDate = m.currentDate.AddDate(0, 0, -7)
			} else if m.viewMode == MonthlyView {
				m.currentDate = m.currentDate.AddDate(0, -1, 0)
			}
			m.dayInput = ""
		case "right", "l":
			if m.viewMode == DailyView {
				m.currentDate = m.currentDate.AddDate(0, 0, 1)
			} else if m.viewMode == WeeklyView {
				m.currentDate = m.currentDate.AddDate(0, 0, 7)
			} else if m.viewMode == MonthlyView {
				m.currentDate = m.currentDate.AddDate(0, 1, 0)
			}
			m.dayInput = ""
		case "t":
			m.currentDate = time.Now()
			m.dayInput = ""
		case "d":
			m.viewMode = DailyView
			m.dayInput = ""
		case "w":
			m.viewMode = WeeklyView
			m.dayInput = ""
		case "m":
			m.viewMode = MonthlyView
			m.dayInput = ""
		case "enter":
			if m.viewMode == MonthlyView && m.dayInput != "" {
				if day, err := strconv.Atoi(m.dayInput); err == nil && day >= 1 && day <= 31 {
					lastDay := time.Date(m.currentDate.Year(), m.currentDate.Month()+1, 0, 0, 0, 0, 0, time.Local).Day()
					if day <= lastDay {
						m.currentDate = time.Date(m.currentDate.Year(), m.currentDate.Month(), day, 0, 0, 0, 0, time.Local)
						m.viewMode = DailyView
						m.dayInput = ""
					}
				}
			}
		case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
			if m.viewMode == MonthlyView {
				m.dayInput += msg.String()
			}
		case "backspace":
			if len(m.dayInput) > 0 {
				m.dayInput = m.dayInput[:len(m.dayInput)-1]
			}
		case "escape":
			m.dayInput = ""
		}
	}
	return m, nil
}

func (m model) handleEventCreationInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.creationMode {
	case NaturalLanguageInput:
		switch msg.String() {
		case "escape":
			m.creationMode = NoCreation
			m.naturalLangInput = ""
			m.message = ""
		case "tab":
			m.creationMode = UIFormInput
			// Initialize form from parsed natural language if possible
			if m.naturalLangInput != "" {
				event, err := parseNaturalLanguage(m.naturalLangInput, m.currentDate)
				if err == nil {
					m.uiFormState = UIFormState{
						summary:     event.Summary,
						description: event.Description,
						date:        event.Start,
						startTime:   event.Start.Format("15:04"),
						endTime:     event.End.Format("15:04"),
					}
				}
			}
		case "enter":
			event, err := parseNaturalLanguage(m.naturalLangInput, m.currentDate)
			if err == nil {
				// Set calendar
				event.CalendarName = m.selectedCalendar
				if color, ok := m.calendars[m.selectedCalendar]; ok {
					event.CalendarColor = color
				} else {
					// Use first available color
					for _, c := range m.calendars {
						event.CalendarColor = c
						break
					}
				}

				// Save to Radicale if configured
				if m.radicaleConfig != nil && m.calendarURLs[m.selectedCalendar] != "" {
					if err := createEventOnRadicale(m.calendarURLs[m.selectedCalendar], event, m.radicaleConfig); err != nil {
						m.message = fmt.Sprintf("Error: %v", err)
					} else {
						m.message = "Event created successfully!"
						m.events = append(m.events, *event)
						m.creationMode = NoCreation
						m.naturalLangInput = ""
					}
				} else {
					// Save locally
					m.events = append(m.events, *event)
					m.message = "Event created successfully!"
					m.creationMode = NoCreation
					m.naturalLangInput = ""
				}
			} else {
				m.message = fmt.Sprintf("Parse error: %v", err)
			}
		case "backspace":
			if len(m.naturalLangInput) > 0 {
				m.naturalLangInput = m.naturalLangInput[:len(m.naturalLangInput)-1]
			}
		default:
			if len(msg.Runes) > 0 {
				m.naturalLangInput += string(msg.Runes)
			}
		}

	case UIFormInput:
		if m.uiFormState.editing {
			// Handle editing mode
			switch msg.String() {
			case "enter":
				// Save current field
				switch m.uiFormState.fieldIndex {
				case 0: // Summary
					m.uiFormState.summary = m.uiFormState.editBuffer
				case 1: // Description
					m.uiFormState.description = m.uiFormState.editBuffer
				case 2: // Date
					if t, err := time.Parse("2006-01-02", m.uiFormState.editBuffer); err == nil {
						m.uiFormState.date = t
					}
				case 3: // Start time
					if _, err := time.Parse("15:04", m.uiFormState.editBuffer); err == nil {
						m.uiFormState.startTime = m.uiFormState.editBuffer
					}
				case 4: // End time
					if _, err := time.Parse("15:04", m.uiFormState.editBuffer); err == nil {
						m.uiFormState.endTime = m.uiFormState.editBuffer
					}
				case 5: // Calendar - cycle through
					calNames := make([]string, 0, len(m.calendars))
					for name := range m.calendars {
						calNames = append(calNames, name)
					}
					sort.Strings(calNames)
					for i, name := range calNames {
						if name == m.selectedCalendar {
							if i+1 < len(calNames) {
								m.selectedCalendar = calNames[i+1]
							} else {
								m.selectedCalendar = calNames[0]
							}
							break
						}
					}
				}
				m.uiFormState.editing = false
				m.uiFormState.editBuffer = ""
			case "escape":
				m.uiFormState.editing = false
				m.uiFormState.editBuffer = ""
			case "backspace":
				if len(m.uiFormState.editBuffer) > 0 {
					m.uiFormState.editBuffer = m.uiFormState.editBuffer[:len(m.uiFormState.editBuffer)-1]
				}
			default:
				if len(msg.Runes) > 0 {
					m.uiFormState.editBuffer += string(msg.Runes)
				}
			}
		} else {
			// Handle navigation mode
			switch msg.String() {
			case "escape":
				m.creationMode = NoCreation
				m.message = ""
			case "tab":
				// Disabled: natural language mode
				// m.creationMode = NaturalLanguageInput
				// m.naturalLangInput = ""
			case "up", "k":
				if m.uiFormState.fieldIndex > 0 {
					m.uiFormState.fieldIndex--
				}
			case "down", "j":
				if m.uiFormState.fieldIndex < 5 {
					m.uiFormState.fieldIndex++
				}
			case "enter":
				// Start editing current field
				m.uiFormState.editing = true
				switch m.uiFormState.fieldIndex {
				case 0:
					m.uiFormState.editBuffer = m.uiFormState.summary
				case 1:
					m.uiFormState.editBuffer = m.uiFormState.description
				case 2:
					m.uiFormState.editBuffer = m.uiFormState.date.Format("2006-01-02")
				case 3:
					m.uiFormState.editBuffer = m.uiFormState.startTime
				case 4:
					m.uiFormState.editBuffer = m.uiFormState.endTime
				case 5:
					// Calendar selection - just cycle, no editing
					calNames := make([]string, 0, len(m.calendars))
					for name := range m.calendars {
						calNames = append(calNames, name)
					}
					sort.Strings(calNames)
					for i, name := range calNames {
						if name == m.selectedCalendar {
							if i+1 < len(calNames) {
								m.selectedCalendar = calNames[i+1]
							} else {
								m.selectedCalendar = calNames[0]
							}
							break
						}
					}
					m.uiFormState.editing = false
				}
			case "s": // Save event
				// Parse start and end times
				startTime, err1 := time.Parse("15:04", m.uiFormState.startTime)
				endTime, err2 := time.Parse("15:04", m.uiFormState.endTime)
				if err1 != nil || err2 != nil {
					m.message = "Invalid time format (use HH:MM)"
					return m, nil
				}

				start := time.Date(m.uiFormState.date.Year(), m.uiFormState.date.Month(), m.uiFormState.date.Day(),
					startTime.Hour(), startTime.Minute(), 0, 0, m.uiFormState.date.Location())
				end := time.Date(m.uiFormState.date.Year(), m.uiFormState.date.Month(), m.uiFormState.date.Day(),
					endTime.Hour(), endTime.Minute(), 0, 0, m.uiFormState.date.Location())

				if end.Before(start) || end.Equal(start) {
					m.message = "End time must be after start time"
					return m, nil
				}

				event := &Event{
					Summary:      m.uiFormState.summary,
					Description:  m.uiFormState.description,
					Start:        start,
					End:          end,
					CalendarName: m.selectedCalendar,
				}

				if color, ok := m.calendars[m.selectedCalendar]; ok {
					event.CalendarColor = color
				}

				// Save to Radicale if configured
				if m.radicaleConfig != nil && m.calendarURLs[m.selectedCalendar] != "" {
					if err := createEventOnRadicale(m.calendarURLs[m.selectedCalendar], event, m.radicaleConfig); err != nil {
						m.message = fmt.Sprintf("Error: %v", err)
					} else {
						m.message = "Event created successfully!"
						m.events = append(m.events, *event)
						m.creationMode = NoCreation
					}
				} else {
					// Save locally
					m.events = append(m.events, *event)
					m.message = "Event created successfully!"
					m.creationMode = NoCreation
				}
			}
		}
	}
	return m, nil
}
func (m model) View() string {
	// Render loading view if loading
	if m.isLoading {
		return m.viewLoading()
	}

	// Render form view if creating event
	if m.creationMode == UIFormInput && m.eventForm != nil {
		return m.viewEventForm()
	}

	// Render natural language input view
	if m.creationMode == NaturalLanguageInput {
		return m.viewNaturalLanguage()
	}

	// Render main calendar view
	switch m.viewMode {
	case DailyView:
		return m.viewDaily()
	case WeeklyView:
		return m.viewWeekly()
	case MonthlyView:
		return m.viewMonthly()
	default:
		return ""
	}
}
