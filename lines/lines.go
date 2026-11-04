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
	Voicemail Action = "voicemail" // play the voicemail greeting, then take a message
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
	// SayAudio is the owner's recording of Say (a prompt file name), played
	// instead of the computer voice when set.
	SayAudio string
}

type Line struct {
	Name     string // the line's address: /call/<Name>
	Business string
	Owner    string // first name, used in "Talk to Joe"
	Greeting string
	// GreetingAudio is the owner's recorded welcome (a prompt file name).
	// When set, callers hear it instead of the greeting and the spoken menu,
	// so the owner says the choices in it.
	GreetingAudio string
	Options       []Option
	// VoicemailText is what callers hear before the beep: on a missed call,
	// while the owner is away, and on a "Take a voicemail" choice. "" means
	// the standard words (DefaultVoicemail).
	VoicemailText string
	// VoicemailAudio is the owner's recording of it (a prompt file name),
	// played instead of the computer voice when set.
	VoicemailAudio string
	// AwayUntil is when the owner's Available switch turns back on (UTC
	// "YYYY-MM-DD HH:MM:SS"); "" means available.
	AwayUntil string
	// Logo is the business's logo (a logo file name), shown to callers
	// instead of its initials; "" means none yet.
	Logo string
	// Premium: the owner's account is on Premium, so calls offer video,
	// screen sharing and the whiteboard, and the line has a meeting room.
	Premium bool
	// Notes: the account is on Premium with "Take notes" on, so callers and guests
	// are told before anything is recorded. Language is the account's: fr or en.
	Notes    bool
	Language string
}

// Notice is what callers and guests are told when a line takes notes, in the
// account's language. Callers hear Call and Message said out loud as well.
type Notice struct {
	Lang       string // what a browser's voice and screen readers need
	Call       string // before the call: the menu itself isn't recorded
	During     string // on the call
	Message    string // before the tone
	Room       string // at the meeting room's door
	RoomDuring string // in the room
}

// Notice is empty when notes are off. A menu that neither rings the owner nor
// takes a message records nothing, so only the room's words are set.
func (l Line) Notice() Notice {
	if !l.Notes {
		return Notice{}
	}
	n := Notice{Lang: "fr-CA",
		Call:       "Votre conversation ou votre message sera enregistré et résumé automatiquement pour " + l.Business + ".",
		During:     "Cet appel est enregistré et résumé automatiquement pour " + l.Business + ".",
		Message:    "Votre message sera enregistré et résumé automatiquement pour " + l.Business + ".",
		Room:       "Cette réunion sera enregistrée et résumée automatiquement pour " + l.Business + ".",
		RoomDuring: "Cette réunion est enregistrée et résumée automatiquement pour " + l.Business + ".",
	}
	if l.Language == "en" {
		n = Notice{Lang: "en-US",
			Call:       "Your conversation or message will be recorded and summarized automatically for " + l.Business + ".",
			During:     "This call is being recorded and summarized automatically for " + l.Business + ".",
			Message:    "Your message will be recorded and summarized automatically for " + l.Business + ".",
			Room:       "This meeting will be recorded and summarized automatically for " + l.Business + ".",
			RoomDuring: "This meeting is being recorded and summarized automatically for " + l.Business + ".",
		}
	}
	if !l.records() {
		n.Call, n.During, n.Message = "", "", ""
	}
	return n
}

func (l Line) records() bool {
	for _, o := range l.Options {
		if o.Action == Ring || o.Action == Voicemail {
			return true
		}
	}
	return false
}

// Rings says whether a choice of the menu rings the owner's phones.
func (l Line) Rings() bool {
	for _, o := range l.Options {
		if o.Action == Ring {
			return true
		}
	}
	return false
}

// A new line starts with this menu (store.CreateLine): a greeting and one
// choice that rings.
const (
	StarterChoice = "Talk to us"
	StarterSay    = "Please hold, I'm connecting you."
)

func StarterGreeting(business string) string { return "Hi, you've reached " + business + "." }

// Starter says whether the owner hasn't touched the starter menu yet.
func (l Line) Starter() bool {
	return l.GreetingAudio == "" && l.Greeting == StarterGreeting(l.Business) &&
		l.VoicemailText == "" && l.VoicemailAudio == "" &&
		len(l.Options) == 1 && l.Options[0].Label == StarterChoice && l.Options[0].SayAudio == ""
}

// DefaultVoicemail is the voicemail greeting until the owner writes their own.
func (l Line) DefaultVoicemail() string {
	who := l.Owner
	if who == "" {
		who = l.Business
	}
	return who + " can't answer right now. After the tone, please say your name and phone number."
}

// VoicemailWords is what callers hear before the beep.
func (l Line) VoicemailWords() string {
	if l.VoicemailText != "" {
		return l.VoicemailText
	}
	return l.DefaultVoicemail()
}

// WelcomeWords is the greeting and the spoken menu: what a recorded welcome
// has to say, since it replaces both. Mirrored in assets/js/menu.js.
func (l Line) WelcomeWords() string {
	return strings.TrimSpace(l.Greeting + " " + l.Menu())
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
// assets/js/menu.js for the "Hear it" preview.
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

// Suggest turns a business name into a line name: "Joe's B&B" -> "joes-bandb".
func Suggest(business string) string {
	s := strings.ToLower(business)
	s = strings.NewReplacer("'", "", "’", "", "&", "and", "é", "e", "è", "e", "ê", "e", "à", "a", "ç", "c", "ô", "o", "î", "i", "û", "u").Replace(s)
	var b strings.Builder
	dash := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 30 {
		out = strings.Trim(out[:30], "-")
	}
	for len(out) < 3 {
		out += "-line"
		out = strings.Trim(out, "-")
	}
	return out
}

var nameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{1,28}[a-z0-9])$`)

// Reserved names would clash with the site's own pages.
var reserved = map[string]bool{"app": true, "call": true, "ws": true, "login": true, "logout": true, "signup": true, "dashboard": true, "assets": true, "admin": true, "api": true,
	"room": true, "logo": true, "prompt": true, "voicemail": true, "download": true, "minivoip": true, "minipass": true, "minicrm": true, "support": true, "help": true, "www": true, "mail": true}

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
