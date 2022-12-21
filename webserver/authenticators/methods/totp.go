package methods

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image/png"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/NHAS/wag/config"
	"github.com/NHAS/wag/data"
	"github.com/NHAS/wag/router"
	"github.com/NHAS/wag/users"
	"github.com/NHAS/wag/utils"
	"github.com/NHAS/wag/webserver/authenticators"
	"github.com/NHAS/wag/webserver/resources"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

type entry struct {
	usetime time.Time
	code    string
}

// Make sure that one time passwords (OTPs) are truly one time, store used codes
var lockULock sync.Mutex
var usedCodes = map[string]entry{}

type Totp struct {
}

func (t *Totp) Init(settings map[string]string) error {
	return nil
}

func (t *Totp) Type() string {
	return authenticators.TotpMFA
}

func (t *Totp) FriendlyName() string {
	return "Time Based Code"
}

func (t *Totp) RegistrationEndpoint(w http.ResponseWriter, r *http.Request) {
	clientTunnelIp := utils.GetIPFromRequest(r)

	if router.IsAuthed(clientTunnelIp.String()) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.Write([]byte(resources.MfaSuccess))
		return
	}

	user, err := users.GetUserFromAddress(clientTunnelIp)
	if err != nil {
		log.Println("unknown", clientTunnelIp, "could not get associated device:", err)
		http.Error(w, "Bad request", 400)
		return
	}

	if user.IsEnforcingMFA() {
		log.Println(user.Username, clientTunnelIp, "tried to re-register mfa despite already being registered")

		http.Error(w, "Bad request", 400)
		return
	}

	switch r.Method {
	case "GET":

		key, err := totp.Generate(totp.GenerateOpts{
			Issuer:      config.Values().Issuer,
			AccountName: user.Username,
		})
		if err != nil {
			log.Println(user.Username, clientTunnelIp, "generate key failed:", err)
			http.Error(w, "Unknown error", 500)
			return
		}

		err = data.SetUserMfa(user.Username, key.URL(), authenticators.TotpMFA)
		if err != nil {
			log.Println(user.Username, clientTunnelIp, "unable to save totp key to db:", err)
			http.Error(w, "Unknown error", 500)
			return
		}

		image, err := key.Image(200, 200)
		if err != nil {
			log.Println(user.Username, clientTunnelIp, "generating image failed:", err)
			http.Error(w, "Unknown error", 500)
			return
		}

		var buff bytes.Buffer
		err = png.Encode(&buff, image)
		if err != nil {
			log.Println(user.Username, clientTunnelIp, "encoding mfa secret as png failed:", err)
			http.Error(w, "Unknown error", 500)
			return
		}

		var mfa = struct {
			ImageData   string
			Key         string
			AccountName string
		}{
			ImageData:   "data:image/png;base64, " + base64.StdEncoding.EncodeToString(buff.Bytes()),
			Key:         key.Secret(),
			AccountName: key.AccountName(),
		}

		jsonResponse(w, &mfa, 200)

	case "POST":
		err = user.Authenticate(clientTunnelIp.String(), t.Type(), t.AuthoriseFunc(w, r))
		msg, status := resultMessage(err)
		jsonResponse(w, msg, status)

		if err != nil {
			log.Println(user.Username, clientTunnelIp, "failed to authorise: ", err.Error())
			return
		}

		log.Println(user.Username, clientTunnelIp, "authorised")
		user.EnforceMFA()

	default:
		http.NotFound(w, r)
		return
	}
}

func (t *Totp) AuthorisationEndpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.NotFound(w, r)
		return
	}

	clientTunnelIp := utils.GetIPFromRequest(r)

	if router.IsAuthed(clientTunnelIp.String()) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.Write([]byte(resources.MfaSuccess))
		return
	}

	user, err := users.GetUserFromAddress(clientTunnelIp)
	if err != nil {
		log.Println("unknown", clientTunnelIp, "could not get associated device:", err)
		http.Error(w, "Bad request", 400)
		return
	}

	if !user.IsEnforcingMFA() {
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	err = user.Authenticate(clientTunnelIp.String(), t.Type(), t.AuthoriseFunc(w, r))

	msg, status := resultMessage(err)
	jsonResponse(w, msg, status)

	if err != nil {
		log.Println(user.Username, clientTunnelIp, "failed to authorise: ", err.Error())
		return
	}

	log.Println(user.Username, clientTunnelIp, "authorised")

}

func (t *Totp) AuthoriseFunc(w http.ResponseWriter, r *http.Request) authenticators.AuthenticatorFunc {
	return func(mfaSecret, username string) error {
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Bad request", 400)
			return err
		}

		code := r.FormValue("code")

		key, err := otp.NewKeyFromURL(mfaSecret)
		if err != nil {
			return err
		}

		if !totp.Validate(code, key.Secret()) {
			return errors.New("code does not match expected")
		}

		lockULock.Lock()
		defer lockULock.Unlock()

		e := usedCodes[username]
		if e.code == code && e.usetime.Add(30*time.Second).After(time.Now()) {
			return errors.New("code already used")
		}

		usedCodes[username] = entry{code: code, usetime: time.Now()}

		return nil
	}
}

func (t *Totp) PromptHandler(w http.ResponseWriter, r *http.Request, username, ip string) {
	if err := renderTemplate(w, resources.TotpMFAPromptTmpl, "", ""); err != nil {
		log.Println(username, ip, "unable to render totp prompt template: ", err)
	}
}

func (t *Totp) RegistrationHandler(w http.ResponseWriter, r *http.Request, username, ip string) {
	if err := renderTemplate(w, resources.TotpMFATemplate, "", ""); err != nil {
		log.Println(username, ip, "unable to render totp mfa template: ", err)
	}
}