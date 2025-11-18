package main

import (
	"encoding/xml"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

type ViewMode int

const (
	DailyView ViewMode = iota
	WeeklyView
	MonthlyView
)

type EventCreationMode int

const (
	NoCreation EventCreationMode = iota
	NaturalLanguageInput
	UIFormInput
)

type loadingMsg struct {
	progress float64
	message  string
}

type loadingCompleteMsg struct{}

type Event struct {
	Summary       string
	Start         time.Time
	End           time.Time
	Description   string
	CalendarName  string
	CalendarColor lipgloss.Color
	UID           string // For Radicale sync
}

type CalendarConfig struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
	File string `json:"file,omitempty"`
	Type string `json:"type,omitempty"` // "radicale", "url", "file", or empty for auto-detect
}

type RadicaleConfig struct {
	ServerURL string `json:"server_url"`
	Username  string `json:"username"`
	Password  string `json:"password"`
}

type Config struct {
	Radicale       *RadicaleConfig  `json:"radicale,omitempty"`
	Calendars      []CalendarConfig `json:"calendars"`
	LocalCalendars []string         `json:"local_calendars,omitempty"`
}

type CalDAVCalendar struct {
	DisplayName string
	URL         string
}

// CalDAV XML structures
type propfindRequest struct {
	XMLName xml.Name `xml:"DAV: propfind"`
	Prop    prop     `xml:"DAV: prop"`
}

type prop struct {
	DisplayName         string `xml:"DAV: displayname"`
	CalendarDescription string `xml:"urn:ietf:params:xml:ns:caldav calendar-description"`
	CalendarColor       string `xml:"http://apple.com/ns/ical/ calendar-color"`
}

type multistatus struct {
	XMLName  xml.Name   `xml:"DAV: multistatus"`
	Response []response `xml:"DAV: response"`
}

type response struct {
	Href     string     `xml:"DAV: href"`
	Propstat []propstat `xml:"DAV: propstat"`
}

type propstat struct {
	Status string `xml:"DAV: status"`
	Prop   prop   `xml:"DAV: prop"`
}

type UIFormState struct {
	summary     string
	description string
	date        time.Time
	startTime   string
	endTime     string
	fieldIndex  int // 0=summary, 1=description, 2=date, 3=start, 4=end, 5=calendar
	editing     bool
	editBuffer  string
}

type model struct {
	events           []Event
	calendars        map[string]lipgloss.Color
	calendarURLs     map[string]string // Map calendar name to Radicale URL
	currentDate      time.Time
	viewMode         ViewMode
	dayInput         string
	width            int
	height           int
	oneShot          bool
	err              error
	radicaleConfig   *RadicaleConfig
	creationMode     EventCreationMode
	naturalLangInput string
	uiFormState      UIFormState
	selectedCalendar string
	message          string // Success/error messages

	// New UI components
	eventForm       *huh.Form
	loadingProgress progress.Model
	isLoading       bool
	loadingMessage  string

	// Form data (pointers for huh form)
	formSummary       *string
	formDescription   *string
	formDate          *string
	formStartTime     *string
	formEndTime       *string
	formCalendar      *string
	formRepeatOptions *string // Single select for repeat option
	formRepeatEndDate *string
	formScrollOffset  int // For scrolling when content is too tall
}
