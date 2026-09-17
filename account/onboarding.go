package account

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html"
	"net/http"
	"strings"
	"sync"
	"time"

	"mu/agent"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/onboarding"
	"mu/internal/origin"
	"mu/internal/phone"
	"mu/internal/thread"
	"mu/service/mail"
	"mu/service/sms"
)

// ChannelWelcome confirms a channel without consuming a proof on a link
// preview. It never signs into or attaches the channel to an existing account.
func ChannelWelcome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid request", 400)
		return
	}
	token := r.Form.Get("token")
	req, err := onboarding.Read(token)
	if err != nil {
		app.Error(w, r, 400, err.Error())
		return
	}
	_, acc := auth.TrySession(r)
	if acc != nil && (req.State == "pending" || acc.ID != req.Account) {
		app.Error(w, r, 409, "You are already signed in. Connect another address or number in Account settings.")
		return
	}
	label := map[string]string{"mail": "email", "sms": "text", "whatsapp": "WhatsApp"}[req.Channel]
	if r.Method == http.MethodPost {
		if r.Header.Get("Origin") != origin.Self() {
			app.Error(w, r, 403, "Reopen the verification link and try again.")
			return
		}
		if r.PostForm.Get("confirm") != "yes" {
			app.Error(w, r, 400, "Confirm to create your account.")
			return
		}
		req, err = onboarding.Confirm(token, func(job *onboarding.Request) error {
			if len(auth.AllAccounts()) == 0 {
				return fmt.Errorf("The operator needs to finish setting up this server first.")
			}
			if job.Channel == "mail" {
				if a, _ := auth.AccountByEmail(job.Address); a != nil {
					return fmt.Errorf("This email already has an account. Sign in to connect it.")
				}
			} else if phone.Owner(job.Address) != "" {
				return fmt.Errorf("This number already has an account. Sign in to connect it.")
			}
			var secret [32]byte
			if _, err := rand.Read(secret[:]); err != nil {
				return err
			}
			a := &auth.Account{ID: job.Account, Name: "Member", Secret: hex.EncodeToString(secret[:]), Created: time.Now()}
			if job.Channel == "mail" {
				a.Email = job.Address
				a.EmailVerified = true
				a.EmailVerifiedAt = time.Now()
			} else {
				a.Approved = true
			}
			if err := auth.CreateMember(a); err != nil {
				return err
			}
			if job.Channel != "mail" {
				return phone.Verify(a.ID, job.Address)
			}
			return nil
		})
		if err != nil {
			app.Error(w, r, 400, err.Error())
			return
		}
		if sess, err := auth.CreateSession(req.Account); err == nil {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: sess.Token, Path: "/", MaxAge: 2592000, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
		}
		fmt.Fprint(w, app.ConsoleHTML("Ready", `<div class="reader-content"><h1>You’re ready</h1><p>Your first request is saved. Micro will reply by `+html.EscapeString(label)+`.</p><p>You can close this page and return to your conversation, or <a href="/account">set up web access</a>.</p></div>`, nil))
		return
	}
	if req.State != "pending" {
		message := "Your account has been created. Return to your conversation for the reply."
		if req.State == "failed" || req.State == "interrupted" || req.State == "confirming" {
			message = "Your first request could not be completed. Check your conversation before sending it again; an action may already have taken place."
		}
		fmt.Fprint(w, app.ConsoleHTML("Getting started", `<div class="reader-content"><h1>Getting started</h1><p>`+html.EscapeString(message)+`</p></div>`, nil))
		return
	}
	fmt.Fprint(w, app.ConsoleHTML("Get started", `<div class="reader-content"><h1>Continue with Micro</h1><p>Confirm that you want an account for `+html.EscapeString(req.Address)+`. Your first request is saved and the reply will come by `+html.EscapeString(label)+`.</p><form method="post" action="/welcome" class="form"><input type="hidden" name="token" value="`+html.EscapeString(token)+`"><label><input type="checkbox" name="confirm" value="yes" required> Create my account. I have read the <a href="/privacy">privacy policy</a>.</label><div class="form-actions"><button type="submit">Continue</button><a href="/login">I already have an account</a></div></form></div>`, nil))
}

var channelWorker sync.Once

func init() { auth.AccountDeleteHooks = append(auth.AccountDeleteHooks, onboarding.Forget) }

// LoadChannelOnboarding resumes only queued requests, never a model/action that
// was interrupted by a server restart. All jobs use the newly verified owner.
func LoadChannelOnboarding() {
	channelWorker.Do(func() {
		if err := onboarding.Recover(); err != nil {
			app.Log("onboarding", "Cannot recover first requests: %v", err)
			return
		}
		go func() {
			for {
				name, job, err := onboarding.Next()
				if err != nil {
					app.Log("onboarding", "Queue unavailable: %v", err)
				}
				if job == nil {
					time.Sleep(5 * time.Second)
					continue
				}
				func() {
					defer func() {
						if recover() != nil {
							app.Log("onboarding", "First request interrupted")
							_ = onboarding.Failed(name, job)
						}
					}()
					client := "mail"
					if job.Channel == "sms" {
						client = thread.SMSClient
					}
					if job.Channel == "whatsapp" {
						client = thread.WhatsAppClient
					}
					conversation := job.MessageID
					if job.Channel != "mail" || conversation == "" {
						conversation = job.Address
					}
					answer, askErr := agent.Ask(agent.AskRequest{Account: job.Account, Client: client, Thread: conversation, Text: job.Text, MessageRef: job.MessageID})
					reply := answer.Text
					if askErr != nil {
						reply = "Your account is ready and your request is saved, but I could not complete it. Please try again in this conversation."
					}
					if job.Channel == "mail" {
						_, err = mail.SendExternalEmail("Micro", mail.SharedAgentAddress(), job.Address, "Re: "+job.Subject, reply, "", job.MessageID)
					} else {
						limit := 460
						if job.Channel == "whatsapp" {
							limit = 1500
						}
						runes := []rune(reply)
						if len(runes) > limit {
							reply = string(runes[:limit-1]) + "…"
						}
						_, err = sms.SendOn(sms.Channel(job.Channel), job.Account, job.Address, strings.TrimSpace(reply))
					}
					if err != nil {
						app.Log("onboarding", "First reply could not be delivered: %v", err)
						_ = onboarding.Failed(name, job)
						return
					}
					if err := onboarding.Complete(name, job); err != nil {
						app.Log("onboarding", "Could not finish request: %v", err)
					}
				}()
			}
		}()
	})
}
