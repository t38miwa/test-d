package getAccessToken

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type TokenResponse struct {
	AccessToken string `json:"access_token"`
}

func GetAccessToken() (string, error) {
	username := os.Getenv("API_USERNAME")
	password := os.Getenv("API_PASSWORD")
	clientID := os.Getenv("API_CLIENT_ID")
	clientSecret := os.Getenv("API_CLIENT_SECRET")

	tlsConfig := &tls.Config{
		MinVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		},
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	payload := strings.NewReader("grant_type=password&username=" + username + "&password=" + password + "&client_id=" + clientID + "&client_secret=" + clientSecret)

	req, err := http.NewRequest("POST", "https://itpv3.transtron.fujitsu.com/oauth2/token", payload)
	if err != nil {
		return "", err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	var tokenResponse TokenResponse
	err = json.Unmarshal(body, &tokenResponse)
	if err != nil {
		return "", err
	}

	if tokenResponse.AccessToken != "" {
		return tokenResponse.AccessToken, nil
	}

	return "", fmt.Errorf("failed to obtain access token")
}