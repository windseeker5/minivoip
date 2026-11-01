package pages

import "minivoip/lines"

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
