package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	deviceCodeURL  = "https://github.com/login/device/code"
	accessTokenURL = "https://github.com/login/oauth/access_token"
)

type DeviceCode struct {
	DeviceCode      string
	UserCode        string
	ExpiresIn       int
	Interval        int
	VerificationURI string
}

type deviceCodeResp struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	VerificationURI string `json:"verification_uri"`
}

type accessTokenResp struct {
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
}

func StartDeviceFlow(ctx context.Context) (DeviceCode, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("scope", "read:user")

	req, _ := http.NewRequestWithContext(ctx, "POST", deviceCodeURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return DeviceCode{}, fmt.Errorf("copilot: device code: %w", err)
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return DeviceCode{}, fmt.Errorf("copilot: device code: unexpected status code: %d", resp.StatusCode)
	}

	var dc deviceCodeResp
	if err := json.NewDecoder(resp.Body).Decode(&dc); err != nil {
		return DeviceCode{}, err
	}

	return DeviceCode{
		DeviceCode:      dc.DeviceCode,
		UserCode:        dc.UserCode,
		ExpiresIn:       dc.ExpiresIn,
		Interval:        dc.Interval,
		VerificationURI: dc.VerificationURI,
	}, nil
}

// PollDeviceFlow: aspetta che l'utente autorizzi. Blocca fino a token, scadenza o errore.
// Ritorna il GitHub OAuth token (da salvare come Credential.Refresh).
func PollDeviceFlow(ctx context.Context, dc DeviceCode) (string, error) {
	interval := time.Duration(dc.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("copilot: device flow scaduto")
		}

		form := url.Values{}
		form.Set("client_id", clientID)
		form.Set("device_code", dc.DeviceCode)
		form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

		req, _ := http.NewRequestWithContext(ctx, "POST", accessTokenURL, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		var at accessTokenResp
		_ = json.NewDecoder(resp.Body).Decode(&at)
		resp.Body.Close()

		switch {
		case at.AccessToken != "":
			return at.AccessToken, nil
		case at.Error == "authorization_pending":
			continue // utente non ha ancora confermato
		case at.Error == "slow_down":
			interval += 5 * time.Second
		case at.Error != "":
			return "", fmt.Errorf("copilot: device flow: %s", at.Error)
		}
	}
}
