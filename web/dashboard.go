package web

import (
	"errors"
	"net/http"
	"strings"

	"minivoip/lines"
	"minivoip/pages"
	"minivoip/store"
)

// baseURL is this site's address as the visitor sees it, for call links.
func baseURL(r *http.Request) string {
	scheme := "http"
	if secure(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request, u store.User) {
	l, err := s.Store.LineForAccount(u.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		render(w, r, pages.NewLine(u.Email, pages.LineForm{}))
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	render(w, r, pages.Dashboard(u.Email, l, baseURL(r)+"/call/"+l.Name, s.Store.NewVoicemails(u.AccountID)))
}

func (s *Server) createLine(w http.ResponseWriter, r *http.Request, u store.User) {
	f := pages.LineForm{
		Name:     strings.ToLower(strings.TrimSpace(r.FormValue("name"))),
		Business: strings.TrimSpace(r.FormValue("business")),
		Owner:    strings.TrimSpace(r.FormValue("owner")),
	}
	if err := lines.CheckName(f.Name); err != nil {
		f.Error = err.Error()
	} else if f.Business == "" || f.Owner == "" {
		f.Error = "Fill in your business name and your first name."
	} else {
		err := s.Store.CreateLine(u.AccountID, f.Name, f.Business, f.Owner)
		switch {
		case errors.Is(err, store.ErrNameTaken):
			f.Error = "Someone already uses " + f.Name + ". Try another name."
		case err != nil:
			serverError(w, r, err)
			return
		default:
			http.Redirect(w, r, "/dashboard/menu", http.StatusSeeOther)
			return
		}
	}
	w.WriteHeader(http.StatusUnprocessableEntity)
	render(w, r, pages.NewLine(u.Email, f))
}

func (s *Server) menuPage(w http.ResponseWriter, r *http.Request, u store.User) {
	l, err := s.Store.LineForAccount(u.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	render(w, r, pages.MenuEditor(u.Email, l, r.URL.Query().Get("saved") != "", ""))
}

// maxOptions keeps the spoken menu short enough to remember.
const maxOptions = 9

func (s *Server) saveMenu(w http.ResponseWriter, r *http.Request, u store.User) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad form", http.StatusBadRequest)
		return
	}
	l, err := s.Store.LineForAccount(u.AccountID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	l.Business = strings.TrimSpace(r.FormValue("business"))
	l.Owner = strings.TrimSpace(r.FormValue("owner"))
	l.Greeting = strings.TrimSpace(r.FormValue("greeting"))
	l.Options = nil
	keys, labels, actions, says, urls := r.Form["key"], r.Form["label"], r.Form["action"], r.Form["say"], r.Form["url"]
	for i := range keys {
		if i >= len(labels) || i >= len(actions) || i >= len(says) || i >= len(urls) {
			break
		}
		o := lines.Option{
			Key:    strings.TrimSpace(keys[i]),
			Label:  strings.TrimSpace(labels[i]),
			Action: lines.Action(actions[i]),
			Say:    strings.TrimSpace(says[i]),
			URL:    strings.TrimSpace(urls[i]),
		}
		if o.Key == "" && o.Label == "" {
			continue // an empty row
		}
		l.Options = append(l.Options, o)
	}

	if msg := checkMenu(l); msg != "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		render(w, r, pages.MenuEditor(u.Email, l, false, msg))
		return
	}
	if err := s.Store.SaveMenu(u.AccountID, l.Business, l.Owner, l.Greeting, l.Options); err != nil {
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/dashboard/menu?saved=1", http.StatusSeeOther)
}

// checkMenu returns what's wrong with a menu, in plain words, or "".
func checkMenu(l lines.Line) string {
	switch {
	case l.Business == "" || l.Owner == "":
		return "Fill in your business name and your first name."
	case l.Greeting == "":
		return "Write a welcome message."
	case len(l.Options) == 0:
		return "Add at least one choice."
	case len(l.Options) > maxOptions:
		return "Use 9 choices or fewer: callers can't remember more."
	}
	seen := map[string]bool{}
	for _, o := range l.Options {
		switch {
		case len(o.Key) != 1 || !strings.Contains("0123456789*#", o.Key):
			return "Each choice needs one key: a number from 0 to 9, * or #."
		case seen[o.Key]:
			return "Key " + o.Key + " is used twice. Give each choice its own key."
		case o.Label == "":
			return "Give choice " + o.Key + " a name, like Bookings."
		case !o.Action.Valid():
			return "Pick what choice " + o.Key + " does."
		case o.Action == lines.Play && o.Say == "":
			return "Write the message that choice " + o.Key + " plays."
		case o.Action == lines.Link && !(strings.HasPrefix(o.URL, "https://") || strings.HasPrefix(o.URL, "http://")):
			return "Choice " + o.Key + " needs a web address starting with https://."
		}
		seen[o.Key] = true
	}
	return ""
}

func (s *Server) phoneApp(w http.ResponseWriter, r *http.Request, u store.User) {
	l, err := s.Store.LineForAccount(u.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	list, err := s.Store.Voicemails(u.AccountID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	render(w, r, pages.Phone(l, list))
}

func (s *Server) callPage(w http.ResponseWriter, r *http.Request) {
	l, err := s.Store.LineByName(r.PathValue("line"))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	render(w, r, pages.Call(l))
}
