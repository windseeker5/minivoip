package pages

import (
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"minivoip/lines"
	"minivoip/store"
)

// HomeTab is what the dashboard's Home tab shows.
type HomeTab struct {
	Email string
	Line  lines.Line
	// Host is this site as the owner sees it: https://voip.dresdell.com.
	Host string
	// Here says the owner can take calls now (web.Server.here).
	Here bool
	// Phones get notifications for this account.
	Phones int
	// Latest calls and voicemails, newest first.
	Latest []store.HistoryEntry
}

func (h HomeTab) callURL() string { return h.Host + "/call/" + h.Line.Name }
func (h HomeTab) roomURL() string { return h.Host + "/room/" + h.Line.Name }

// hostName is the site without https://, for showing the link.
func (h HomeTab) hostName() string {
	return strings.TrimPrefix(strings.TrimPrefix(h.Host, "https://"), "http://")
}

// status says in plain words whether calls reach the owner now; on is the
// green dot.
func (h HomeTab) status() (text string, on bool) {
	switch {
	case !store.IsAvailable(h.Line.AwayUntil):
		return "You're away. Callers hear your menu and can leave a message.", false
	case h.Here:
		return "Your line is on. Calls ring your phone.", true
	case !h.Line.Rings():
		return "Your line is on. No choice in your menu rings you, so callers leave messages.", false
	}
	return "Your line is on. Callers leave messages until your phone is set up.", false
}

// setupStep is one line of "Get set up". Its button opens the window named
// Open, or goes to Href.
type setupStep struct {
	Title, Hint, Action, Href, Open string
	Done                            bool
}

func (h HomeTab) steps() []setupStep {
	return []setupStep{
		{Title: "Your welcome and choices", Hint: "Say hello in your own words and choose what each key does.",
			Action: "Set it up", Href: "/dashboard/menu", Done: !h.Line.Starter()},
		{Title: "Make your phone ring", Hint: "Open the phone app on your phone and turn on notifications.",
			Action: "Set up my phone", Open: "dlg-phone", Done: h.Phones > 0},
		{Title: "Your logo", Hint: "Callers see it on their screen, so they know it's you.",
			Action: "Add my logo", Href: "/dashboard/business", Done: h.Line.Logo != ""},
	}
}

// nextStep is the first step not done yet, and how many are left.
func nextStep(steps []setupStep) (left int, next setupStep) {
	for _, s := range steps {
		if !s.Done {
			if left == 0 {
				next = s
			}
			left++
		}
	}
	return left, next
}

// stepsLeftText is the phone's band: "One step left: make your phone ring."
func stepsLeftText(left int, next setupStep) string {
	t := []rune(next.Title)
	t[0] = unicode.ToLower(t[0])
	if left == 1 {
		return "One step left: " + string(t) + "."
	}
	return strconv.Itoa(left) + " steps left. Next: " + string(t) + "."
}

// latestWhen is the time for a call today, else its day.
func latestWhen(utc string) string {
	if d := dayLabel(utc, time.Now()); d != "Today" {
		return d
	}
	return clock(utc)
}

// firstLetter is the round account button's letter.
func firstLetter(email string) string {
	r, _ := utf8.DecodeRuneInString(email)
	return strings.ToUpper(string(r))
}
