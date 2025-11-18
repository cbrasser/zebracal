package main

import (
	"github.com/charmbracelet/lipgloss"
)

// Color palette for calendars
var calendarColors = []lipgloss.Color{
	lipgloss.Color("205"), // Pink
	lipgloss.Color("117"), // Light Blue
	lipgloss.Color("229"), // Yellow
	lipgloss.Color("120"), // Green
	lipgloss.Color("183"), // Purple
	lipgloss.Color("216"), // Peach
	lipgloss.Color("86"),  // Cyan
	lipgloss.Color("211"), // Light Pink
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86")).
			Padding(0, 1)

	dateHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("117")).
			Padding(0, 1).
			MarginTop(1).
			MarginBottom(1)

	timeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Bold(true)

	noEventsStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Italic(true).
			Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1).
			Padding(0, 1)

	calendarLabelStyle = lipgloss.NewStyle().
				Padding(0, 1).
				MarginTop(1)

	eventBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1).
			MarginBottom(0)

	cellStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			Width(10).
			Height(5).
			Padding(0, 1)

	todayCellStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("205")).
			Width(10).
			Height(5).
			Padding(0, 1)

	weekdayHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("117")).
				Width(12).
				Align(lipgloss.Center)

	inputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("117")).
			Bold(true)

	fieldLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	selectedFieldStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("117")).
				Bold(true)

	summaryStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(1, 2).
			Width(30)
)
