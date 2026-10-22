// Package lines describes a phone line and its voice menu. The store package
// saves them; the call pages play them.
package lines

import (
	"errors"
	"regexp"
	"strings"
)

// Action is what a menu choice does (docs/architecture.md, "The voice menu").
type Action string

const (
	Ring      Action = "ring"      // ring the owner's phones
	Play      Action = "play"      // say a message, then hang up
	Link      Action = "link"      // open a web page
	Voicemail Action = "voicemail" // take a message (not built yet: says so)
)

// Actions in the order the menu editor lists them, with their labels.
var Actions = []struct {
	Value Action
	Label string
}{
	{Ring, "Ring my phone"},
	{Play, "Play a message"},
	{Link, "Open a web page"},
	{Voicemail, "Take a voicemail"},
}

func (a Action) Valid() bool {
	for _, x := range Actions {
		if x.Value == a {
			return true
		}
	}
	return false
}

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
	if len(l.Options) == 0 {
		return ""
	}
	parts := make([]string, len(l.Options))
	for i, o := range l.Options {
		parts[i] = "press " + o.Key + " " + choicePhrase(o.Label)
	}
	s := strings.Join(parts, ", ") + "."
	return strings.ToUpper(s[:1]) + s[1:]
}

// choicePhrase turns a choice name into speech: "Talk to Joe" becomes
// "to talk to Joe", "Bookings" becomes "for bookings". Mirrored in
// assets/js/dashboard.js for the "Hear it" preview.
func choicePhrase(label string) string {
	s := strings.ToLower(label[:1]) + label[1:]
	first := strings.Fields(s)[0]
	if verbs[first] {
		return "to " + s
	}
	return "for " + s
}

var verbs = map[string]bool{"talk": true, "speak": true, "leave": true, "book": true, "hear": true, "reach": true,
	"get": true, "visit": true, "see": true, "check": true, "make": true, "order": true, "ask": true, "call": true}

var nameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{1,28}[a-z0-9])$`)

// Reserved names would clash with the site's own pages.
var reserved = map[string]bool{"app": true, "call": true, "ws": true, "login": true, "logout": true, "signup": true, "dashboard": true, "assets": true, "admin": true, "api": true}

// CheckName says why a line name can't be used, or nil.
func CheckName(name string) error {
	if !nameRe.MatchString(name) {
		return errors.New("Use 3 to 30 lowercase letters, numbers or dashes, like joesbnb or joes-bnb.")
	}
	if reserved[name] {
		return errors.New("That name is reserved. Pick another one.")
	}
	return nil
}
