package app

import "time"

// useAgencyTime gives the modules that need it the agency's time zone
// (HP_TIME_ZONE): what day it is, and the dates in exports; and the company
// exports the words for Settings' lists.
func (m *Modules) useAgencyTime(loc *time.Location) {
	m.Companies.UseLabels(m.Settings)
	m.Candidates.UseTimeZone(loc) // a call's date is the agency's day
	m.Jobs.UseTimeZone(loc)       // intake dates and ad closing days are the agency's
	m.Companies.UseTimeZone(loc)  // dates in exports
	m.Contacts.UseTimeZone(loc)   // dates in exports
}
