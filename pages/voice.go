package pages

import (
	"strings"

	"minivoip/lines"
)

// exampleLine stands in on the landing page when minivoip's own line doesn't
// exist (tests, a fresh install). It is the menu Ken plans for his line.
var exampleLine = lines.Line{
	Greeting: "Hi, you've reached minivoip.",
	Options: []lines.Option{
		{Key: "1", Label: "Book a demo", Action: lines.Ring},
		{Key: "2", Label: "How it works and prices", Action: lines.Link},
		{Key: "0", Label: "Leave a message", Action: lines.Voicemail},
	},
}

// voiceWord is one word of what callers hear, written under the voice wave.
type voiceWord struct {
	Text  string
	Class string // "said", "now" or ""
}

// voiceWords splits the greeting and the spoken menu into words. At rest the
// greeting reads as said and the first choice as being said, like the
// recording halfway through; landing.js moves both while it plays.
func voiceWords(l lines.Line) []voiceWord {
	var out []voiceWord
	for _, w := range strings.Fields(l.Greeting) {
		out = append(out, voiceWord{w + " ", "said"})
	}
	class := "now"
	for _, w := range strings.Fields(l.Menu()) {
		out = append(out, voiceWord{w + " ", class})
		if strings.HasSuffix(w, ",") || strings.HasSuffix(w, ".") {
			class = ""
		}
	}
	if len(out) > 0 {
		out[0].Text = "“" + out[0].Text
		last := &out[len(out)-1]
		last.Text = strings.TrimSpace(last.Text) + "”"
	}
	return out
}

// ringChoice is the choice shown on the owner's ringing phone: the line's
// first choice that rings.
func ringChoice(l lines.Line) string {
	for _, o := range l.Options {
		if o.Action == lines.Ring {
			return o.Label
		}
	}
	if len(l.Options) > 0 {
		return l.Options[0].Label
	}
	return "Talk to us"
}
