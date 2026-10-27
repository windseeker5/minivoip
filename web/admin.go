package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"minivoip/pages"
	"minivoip/store"
)

// isAdmin: users marked in the database, or listed in Server.Admins.
func (s *Server) isAdmin(u store.User) bool {
	if u.IsAdmin {
		return true
	}
	for _, a := range s.Admins {
		if strings.EqualFold(strings.TrimSpace(a), u.Email) {
			return true
		}
	}
	return false
}

// admin is auth plus "admins only"; others get a plain 404.
func (s *Server) admin(next func(http.ResponseWriter, *http.Request, store.User)) http.Handler {
	return s.auth(func(w http.ResponseWriter, r *http.Request, u store.User) {
		if !u.IsAdmin {
			http.NotFound(w, r)
			return
		}
		next(w, r, u)
	})
}

func (s *Server) promoPage(w http.ResponseWriter, r *http.Request, u store.User) {
	s.renderPromo(w, r, u, pages.PromoForm{}, r.URL.Query().Get("saved"))
}

func (s *Server) renderPromo(w http.ResponseWriter, r *http.Request, u store.User, f pages.PromoForm, saved string) {
	list, err := s.Store.PromoCodes()
	if err != nil {
		serverError(w, r, err)
		return
	}
	accounts, err := s.Store.Accounts()
	if err != nil {
		serverError(w, r, err)
		return
	}
	if f.Error != "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	render(w, r, pages.PromoPage(u.Email, list, accounts, f, saved))
}

func (s *Server) createPromo(w http.ResponseWriter, r *http.Request, u store.User) {
	f := pages.PromoForm{
		Code:      strings.ToUpper(strings.TrimSpace(r.FormValue("code"))),
		MaxUses:   strings.TrimSpace(r.FormValue("max_uses")),
		ValidFrom: strings.TrimSpace(r.FormValue("valid_from")),
		ValidTo:   strings.TrimSpace(r.FormValue("valid_to")),
		Plan:      r.FormValue("plan"),
	}
	max := 0
	if f.MaxUses != "" {
		n, err := strconv.Atoi(f.MaxUses)
		if err != nil || n < 0 {
			f.Error = "\"How many sign ups\" must be a number, or empty for no limit."
		}
		max = n
	}
	for _, d := range []string{f.ValidFrom, f.ValidTo} {
		if _, err := time.Parse(time.DateOnly, d); d != "" && err != nil && f.Error == "" {
			f.Error = "Dates must look like 2026-12-31."
		}
	}
	if f.Error == "" && f.ValidFrom != "" && f.ValidTo != "" && f.ValidTo < f.ValidFrom {
		f.Error = "\"Usable until\" is before \"Usable from\"."
	}
	if f.Error == "" && !store.Plan(f.Plan).Valid() {
		f.Error = "Pick the plan the code gives: Basic or Premium."
	}
	if f.Error == "" {
		if err := store.CheckPromoCode(f.Code); err != nil {
			f.Error = err.Error()
		}
	}
	if f.Error == "" {
		err := s.Store.CreatePromoCode(f.Code, store.Plan(f.Plan), max, f.ValidFrom, f.ValidTo)
		switch {
		case errors.Is(err, store.ErrPromoExists):
			f.Error = "The code " + f.Code + " already exists."
		case err != nil:
			serverError(w, r, err)
			return
		default:
			http.Redirect(w, r, "/admin/promo?saved="+url.QueryEscape(f.Code+" is ready: it gives "+store.Plan(f.Plan).Name()+". Share it with the people you choose."), http.StatusSeeOther)
			return
		}
	}
	s.renderPromo(w, r, u, f, "")
}

func (s *Server) revokePromo(w http.ResponseWriter, r *http.Request, u store.User) {
	code := r.PathValue("code")
	revoked := r.FormValue("revoked") == "1"
	err := s.Store.SetPromoRevoked(code, revoked)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	msg := code + " is active again."
	if revoked {
		msg = code + " is revoked: nobody can sign up with it, and its accounts are no longer free forever."
	}
	http.Redirect(w, r, "/admin/promo?saved="+url.QueryEscape(msg), http.StatusSeeOther)
}

// setPlan moves an account to Basic or Premium (the admin page's Accounts).
func (s *Server) setPlan(w http.ResponseWriter, r *http.Request, u store.User) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	plan := store.Plan(r.FormValue("plan"))
	if err != nil || !plan.Valid() {
		http.Error(w, "Pick Basic or Premium.", http.StatusBadRequest)
		return
	}
	err = s.Store.SetPlan(id, plan)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	msg := r.FormValue("who") + " is on " + plan.Name() + " now."
	http.Redirect(w, r, "/admin/promo?saved="+url.QueryEscape(strings.TrimSpace(msg)), http.StatusSeeOther)
}

// planText says what an account gets and pays, for the dashboard.
func (s *Server) planText(accountID int64) string {
	b, err := s.Store.Billing(accountID)
	if err != nil {
		return ""
	}
	plan := "Plan: " + b.Plan.Name()
	switch {
	case b.Code == "":
		return plan + ", free while in beta."
	case b.Revoked:
		return plan + ", free while in beta (the code " + b.Code + " was revoked)."
	}
	return plan + ", free forever with the code " + b.Code + "."
}
