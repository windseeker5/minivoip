package store

// PremiumRequest is someone who asked for Premium on the landing page.
type PremiumRequest struct {
	Email     string
	CreatedAt string
}

// AskPremium saves an email from the landing page's "Get Premium" form.
// Asking twice keeps the first date.
func (s *Store) AskPremium(email string) error {
	_, err := s.db.Exec("INSERT OR IGNORE INTO premium_request (email) VALUES (?)", email)
	return err
}

// PremiumRequests lists them, newest first.
func (s *Store) PremiumRequests() ([]PremiumRequest, error) {
	rows, err := s.db.Query("SELECT email, created_at FROM premium_request ORDER BY created_at DESC, id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PremiumRequest
	for rows.Next() {
		var p PremiumRequest
		if err := rows.Scan(&p.Email, &p.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}
