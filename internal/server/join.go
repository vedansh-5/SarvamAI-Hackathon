package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/vedansh/vans/internal/synth"
)

const (
	EnvJoinFrom    = "VANS_JOIN_FROM"
	EnvJoinDaily   = "VANS_JOIN_DAILY"
	EnvJoinsPerDay = "VANS_JOINS_PER_DAY"

	JoinDaily      = 100
	JoinsPerDay    = 50
	CodeLasts      = 15 * time.Minute
	ResendAfter    = time.Minute
	MostTries      = 5
	MostSends      = 5
	MostFromOneNet = 10
	MostSentADay   = 150
)

var ErrEmailTaken = errors.New("server: that email already has a code")

type Pending struct {
	Email   string
	Hash    string
	Expires time.Time
	SentAt  time.Time
	Tries   int
	Sends   int
	Day     string
}

type Joins interface {
	Joined(ctx context.Context, email string) (Tester, bool, error)
	Join(ctx context.Context, t Tester) error
	Pending(ctx context.Context, email string) (Pending, bool, error)
	SetPending(ctx context.Context, p Pending) error
	Count(ctx context.Context, day, what string) (int, error)
}

type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
}

type Joining struct {
	Joins   Joins
	Send    Sender
	Daily   int
	PerDay  int
	HasMail func(ctx context.Context, domain string) bool
}

func JoiningFromEnv(joins Joins, send Sender) (*Joining, error) {
	if joins == nil || send == nil {
		return nil, nil
	}
	j := &Joining{Joins: joins, Send: send, Daily: JoinDaily, PerDay: JoinsPerDay, HasMail: HasMail}
	for name, into := range map[string]*int{EnvJoinDaily: &j.Daily, EnvJoinsPerDay: &j.PerDay} {
		spelled := strings.TrimSpace(os.Getenv(name))
		if spelled == "" {
			continue
		}
		n, err := strconv.Atoi(spelled)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("%s must be a whole number", name)
		}
		*into = n
	}
	return j, nil
}

func HasMail(ctx context.Context, domain string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	records, err := net.DefaultResolver.LookupMX(ctx, domain)
	var missing *net.DNSError
	if errors.As(err, &missing) && missing.IsNotFound {
		return false
	}
	return err != nil || len(records) > 0
}

func EmailOf(spelled string) (string, error) {
	spelled = strings.TrimSpace(spelled)
	parsed, err := mail.ParseAddress(spelled)
	if err != nil || parsed.Address != spelled || len(spelled) > 254 {
		return "", errors.New("That doesn't look like an email address.")
	}
	email := strings.ToLower(parsed.Address)
	_, domain, _ := strings.Cut(email, "@")
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return "", errors.New("That doesn't look like an email address.")
	}
	if throwaway[domain] {
		return "", errors.New("Use an email you keep; throwaway addresses can't sign up.")
	}
	return email, nil
}

func newDigits() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1_000_000))
	return fmt.Sprintf("%06d", n.Int64())
}

func hashOf(email, code string) string {
	sum := sha256.Sum256([]byte(email + "\n" + code))
	return hex.EncodeToString(sum[:])
}

func networkOf(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func answer(w http.ResponseWriter, v any) {
	encoded, _ := json.Marshal(v)
	reply(w, http.StatusOK, synth.Answered{Answer: encoded})
}

func failed(w http.ResponseWriter, status int, message string) {
	reply(w, status, synth.Answered{Fault: &synth.Fault{Kind: synth.FaultFailed, Message: message}})
}

func readJoin[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var v T
	data, err := io.ReadAll(io.LimitReader(r.Body, 4<<10))
	if err != nil || json.Unmarshal(data, &v) != nil {
		refuse(w, http.StatusBadRequest, "the request is not readable")
		return v, false
	}
	return v, true
}

func (s *Server) joinAsk(w http.ResponseWriter, r *http.Request) {
	j := s.Joining
	if j == nil {
		failed(w, http.StatusServiceUnavailable, "Signing up isn't open yet.")
		return
	}
	asked, ok := readJoin[synth.JoinAsk](w, r)
	if !ok {
		return
	}
	email, err := EmailOf(asked.Email)
	if err != nil {
		refuse(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, now := r.Context(), s.now()
	day := Day(now)
	from, err := j.Joins.Count(ctx, day, "net:"+networkOf(r))
	if err != nil {
		s.logf("could not count sign-ups: %v", err)
		failed(w, http.StatusServiceUnavailable, "VANS could not be reached. Try again in a minute.")
		return
	}
	if from > MostFromOneNet {
		refuse(w, http.StatusTooManyRequests, "Too many sign-ups from this network today. Try again tomorrow.")
		return
	}
	_, domain, _ := strings.Cut(email, "@")
	if j.HasMail != nil && !j.HasMail(ctx, domain) {
		refuse(w, http.StatusBadRequest, "That email's domain can't receive mail. Check the spelling.")
		return
	}
	p, found, err := j.Joins.Pending(ctx, email)
	if err != nil {
		s.logf("could not read a pending sign-up: %v", err)
		failed(w, http.StatusServiceUnavailable, "VANS could not be reached. Try again in a minute.")
		return
	}
	if found && now.Before(p.Expires) && now.Sub(p.SentAt) < ResendAfter {
		answer(w, struct{}{})
		return
	}
	if !found || p.Day != day {
		p = Pending{Email: email, Day: day}
	}
	if p.Sends >= MostSends {
		refuse(w, http.StatusTooManyRequests, "That's enough codes for this email today. Try again tomorrow.")
		return
	}
	sent, err := j.Joins.Count(ctx, day, "sent")
	if err != nil {
		s.logf("could not count codes sent: %v", err)
		failed(w, http.StatusServiceUnavailable, "VANS could not be reached. Try again in a minute.")
		return
	}
	if sent > MostSentADay {
		s.logf("over %d sign-in codes sent today", MostSentADay)
		failed(w, http.StatusServiceUnavailable, "VANS can't send more codes today. Try again tomorrow.")
		return
	}
	code := newDigits()
	p.Hash, p.Expires, p.SentAt, p.Tries, p.Sends = hashOf(email, code), now.Add(CodeLasts), now, 0, p.Sends+1
	if err := j.Joins.SetPending(ctx, p); err != nil {
		s.logf("could not keep a pending sign-up: %v", err)
		failed(w, http.StatusServiceUnavailable, "VANS could not be reached. Try again in a minute.")
		return
	}
	if err := j.Send.Send(ctx, email, "Your VANS code: "+code, codeMail(code)); err != nil {
		s.logf("could not send a sign-in code: %v", err)
		failed(w, http.StatusServiceUnavailable, "VANS could not send the email. Try again in a minute.")
		return
	}
	s.logf("sent a sign-in code (%d today)", sent)
	answer(w, struct{}{})
}

func codeMail(code string) string {
	return "Your code for VANS is\n\n    " + code + "\n\n" +
		"Type it into VANS to finish signing in. It works for 15 minutes.\n\n" +
		"You got this because someone typed this address into VANS. If it wasn't you, ignore this email; nothing else will be sent.\n\n" +
		"VANS · https://summonvans.in\n"
}

func (s *Server) joinCheck(w http.ResponseWriter, r *http.Request) {
	j := s.Joining
	if j == nil {
		failed(w, http.StatusServiceUnavailable, "Signing up isn't open yet.")
		return
	}
	checked, ok := readJoin[synth.JoinCheck](w, r)
	if !ok {
		return
	}
	email, err := EmailOf(checked.Email)
	if err != nil {
		refuse(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, now := r.Context(), s.now()
	p, found, err := j.Joins.Pending(ctx, email)
	if err != nil {
		s.logf("could not read a pending sign-up: %v", err)
		failed(w, http.StatusServiceUnavailable, "VANS could not be reached. Try again in a minute.")
		return
	}
	if !found || p.Hash == "" || !now.Before(p.Expires) {
		refuse(w, http.StatusBadRequest, "That code has run out. Ask for a new one.")
		return
	}
	if p.Tries >= MostTries {
		refuse(w, http.StatusBadRequest, "Too many wrong tries. Ask for a new code.")
		return
	}
	code := strings.Join(strings.Fields(checked.Code), "")
	if subtle.ConstantTimeCompare([]byte(hashOf(email, code)), []byte(p.Hash)) != 1 {
		p.Tries++
		if err := j.Joins.SetPending(ctx, p); err != nil {
			s.logf("could not count a wrong try: %v", err)
		}
		refuse(w, http.StatusBadRequest, "That code isn't right. Check the email and try again.")
		return
	}
	p.Hash, p.Expires = "", time.Time{}
	if err := j.Joins.SetPending(ctx, p); err != nil {
		s.logf("could not close a pending sign-up: %v", err)
	}

	who, found, err := j.Joins.Joined(ctx, email)
	if err != nil {
		s.logf("could not look up a joined email: %v", err)
		failed(w, http.StatusServiceUnavailable, "VANS could not be reached. Try again in a minute.")
		return
	}
	if found {
		s.logf("%s signed in again", who.Name)
		answer(w, synth.Joined{Invite: who.Code, Waiting: who.Waiting})
		return
	}
	joined, err := j.Joins.Count(ctx, Day(now), "joined")
	if err != nil {
		s.logf("could not count new testers: %v", err)
		failed(w, http.StatusServiceUnavailable, "VANS could not be reached. Try again in a minute.")
		return
	}
	waiting := j.PerDay > 0 && joined > j.PerDay
	made := Tester{Name: email, Email: email, Code: NewCode(), Daily: j.Daily, Off: waiting, Waiting: waiting}
	if err := j.Joins.Join(ctx, made); err != nil {
		if errors.Is(err, ErrEmailTaken) {
			if again, ok, _ := j.Joins.Joined(ctx, email); ok {
				answer(w, synth.Joined{Invite: again.Code, Waiting: again.Waiting})
				return
			}
		}
		s.logf("could not make a tester for a new sign-up: %v", err)
		failed(w, http.StatusServiceUnavailable, "VANS could not be reached. Try again in a minute.")
		return
	}
	state := "joined"
	if waiting {
		state = "joined the waiting list"
	}
	s.logf("%s %s", email, state)
	if s.Mail != nil {
		if err := s.Mail.Mail(ctx, "VANS: "+email+" "+state, email+" "+state+" from the app, with "+strconv.Itoa(j.Daily)+" requests a day."); err != nil {
			s.logf("could not mail about a new tester: %v", err)
		}
	}
	answer(w, synth.Joined{Invite: made.Code, Waiting: waiting})
}

var throwaway = map[string]bool{
	"mailinator.com": true, "guerrillamail.com": true, "guerrillamail.net": true, "guerrillamail.org": true,
	"guerrillamailblock.com": true, "sharklasers.com": true, "grr.la": true, "pokemail.net": true, "spam4.me": true,
	"10minutemail.com": true, "10minutemail.net": true, "20minutemail.com": true, "temp-mail.org": true,
	"temp-mail.io": true, "tempmail.com": true, "tempmail.net": true, "tempmailo.com": true, "tempail.com": true,
	"tempr.email": true, "tmpmail.org": true, "tmpmail.net": true, "yopmail.com": true, "yopmail.net": true,
	"yopmail.fr": true, "trashmail.com": true, "trashmail.net": true, "trashmail.de": true, "getnada.com": true,
	"nada.email": true, "dispostable.com": true, "maildrop.cc": true, "throwawaymail.com": true,
	"fakeinbox.com": true, "mintemail.com": true, "mohmal.com": true, "emailondeck.com": true, "moakt.com": true,
	"burnermail.io": true, "spamgourmet.com": true, "mailnesia.com": true, "mytemp.email": true,
	"discard.email": true, "mailcatch.com": true, "inboxkitten.com": true, "getairmail.com": true,
	"mailpoof.com": true, "emailfake.com": true, "fakemail.net": true, "spambox.us": true,
	"mailsac.com": true, "harakirimail.com": true, "linshiyouxiang.net": true, "1secmail.com": true,
	"1secmail.org": true, "1secmail.net": true, "dropmail.me": true, "emltmp.com": true, "minuteinbox.com": true,
	"tempinbox.com": true, "spamdecoy.net": true, "mailtemp.net": true, "tempemail.co": true, "byom.de": true,
	"mail.tm": true, "mail.gw": true, "cuvox.de": true, "dayrep.com": true,
	"einrot.com": true, "fleckens.hu": true, "gustr.com": true, "jourrapide.com": true, "rhyta.com": true,
	"superrito.com": true, "teleworm.us": true, "armyspy.com": true,
}
