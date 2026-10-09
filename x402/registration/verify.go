package registration

import (
	"errors"
	"fmt"
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"net/url"
)

func SendVerification(acc *auth.Account, email, origin string) error {
	if app.EmailSender == nil {
		return errors.New("Email verification is not configured; ask the instance operator to approve your account")
	}
	if !app.ValidEmail(email) {
		return errors.New("Please enter a valid email address")
	}
	if err := auth.SetAccountEmail(acc.ID, email); err != nil {
		return err
	}
	token, err := auth.CreateEmailVerificationToken(acc.ID, email)
	if err != nil {
		return err
	}
	link := origin + "/verify?token=" + url.QueryEscape(token)
	plain := fmt.Sprintf("Hi %s,\n\nVerify your account: %s\n\nThis link expires in 24 hours. If you did not request this, ignore this email.", acc.Name, link)
	body := `<p>Verify your account:</p><p><a href="` + html.EscapeString(link) + `">Verify email</a></p><p>This link expires in 24 hours. If you did not request this, ignore this email.</p>`
	if err := app.EmailSender(email, "Verify your account", plain, body, ""); err != nil {
		return errors.New("Could not send verification email. Please try again.")
	}
	return nil
}
