// Package lines holds the phone lines and their voice menus. For the first
// real-call test there is one line, written in code; it moves to SQLite in
// Phase 2 (see docs/plan.md).
package lines

import "strings"

// Action is what a menu choice does (docs/architecture.md, "The voice menu").
type Action string

const (
	Ring Action = "ring" // ring the owner's phones
	Play Action = "play" // say a message, then hang up
	Link Action = "link" // open a web page
)

type Option struct {
	Key    string
	Label  string
	Action Action
	Say    string // spoken when the caller picks this option
	URL    string // for Link
}

type Line struct {
	Name     string // the line's address: /call/<Name>
	Business string
	Owner    string // first name, used in "Talk to Joe"
	Greeting string
	Options  []Option
}

// Menu is the spoken menu: "Press 1 for bookings, press 2 ...".
func (l Line) Menu() string {
	parts := make([]string, len(l.Options))
	for i, o := range l.Options {
		parts[i] = "press " + o.Key + " for " + strings.ToLower(o.Label[:1]) + o.Label[1:]
	}
	s := strings.Join(parts, ", ") + "."
	return strings.ToUpper(s[:1]) + s[1:]
}

var all = map[string]Line{
	"joesbnb": {
		Name:     "joesbnb",
		Business: "Joe's B&B",
		Owner:    "Joe",
		Greeting: "Hi, you've reached Joe's B&B.",
		Options: []Option{
			{Key: "1", Label: "Bookings", Action: Link, Say: "Opening our booking page.", URL: "/"},
			{Key: "2", Label: "Talk to Joe", Action: Ring, Say: "Please hold, I'm ringing Joe."},
			{Key: "3", Label: "Opening hours", Action: Play, Say: "We're open every day from 8 a.m. to 8 p.m. Thanks for calling!"},
		},
	},
}

func Get(name string) (Line, bool) {
	l, ok := all[name]
	return l, ok
}
