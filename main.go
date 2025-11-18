package main

import (
	"flag"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	//TODO: Flag "--tomorrow" -> Show tomorrow at a glance
	nextFlag := flag.Bool("next", false, "Show next upcoming event and quit")
	dayFlag := flag.Bool("day", false, "Show daily view and quit")
	weekFlag := flag.Bool("week", false, "Show weekly view and quit")
	monthFlag := flag.Bool("month", false, "Show monthly view and quit")
	flag.Parse()

	config, _ := loadConfig()
	var radicaleConfig *RadicaleConfig
	if config != nil && config.Radicale != nil {
		radicaleConfig = config.Radicale
	}

	events, calendars, calendarURLs, _ := loadAllCalendars(radicaleConfig)

	if *nextFlag {
		nextEvent := getNextEvent(events)
		fmt.Println(renderNextEvent(nextEvent))
		return
	}

	viewMode := DailyView
	oneShot := false

	if *dayFlag {
		viewMode = DailyView
		oneShot = true
	} else if *weekFlag {
		viewMode = WeeklyView
		oneShot = true
	} else if *monthFlag {
		viewMode = MonthlyView
		oneShot = true
	}

	m := initialModel(viewMode, oneShot, radicaleConfig)
	m.events = events
	m.calendars = calendars
	m.calendarURLs = calendarURLs

	if oneShot {
		fmt.Println(m.View())
		return
	}

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
