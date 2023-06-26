// Copyright 2023 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package cli

import (
	"context"
	stdjson "encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/terramate-io/terramate/cloud"
	"github.com/terramate-io/terramate/cmd/terramate/cli/out"
	"github.com/terramate-io/terramate/errors"
)

const githubOIDCProviderName = "GitHub Actions OIDC"

type githubOIDC struct {
	mu        sync.RWMutex
	token     string
	jwtClaims jwt.MapClaims

	expireAt  time.Time
	repoOwner string
	repoName  string

	reqURL   string
	reqToken string

	output out.O
}

func newGithubOIDC(output out.O) *githubOIDC {
	return &githubOIDC{
		output: output,
	}
}

func (g *githubOIDC) Load() (bool, error) {
	const envReqURL = "ACTIONS_ID_TOKEN_REQUEST_URL"
	const envReqTok = "ACTIONS_ID_TOKEN_REQUEST_TOKEN"

	g.reqURL = os.Getenv(envReqURL)
	if g.reqURL == "" {
		return false, nil
	}

	g.reqToken = os.Getenv(envReqTok)

	audience := oidcAudience()
	if audience != "" {
		u, err := url.Parse(g.reqURL)
		if err != nil {
			return false, errors.E(err, "invalid ACTIONS_ID_TOKEN_REQUEST_URL env var")
		}

		qr := u.Query()
		qr.Set("audience", audience)
		u.RawQuery = qr.Encode()
		g.reqURL = u.String()
	}

	err := g.Refresh()
	return err == nil, err
}

func (g *githubOIDC) Name() string {
	return githubOIDCProviderName
}

func (g *githubOIDC) IsExpired() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return time.Now().After(g.expireAt)
}

func (g *githubOIDC) ExpireAt() time.Time {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.expireAt
}

func (g *githubOIDC) Refresh() error {
	const oidcTimeout = 3 // seconds
	ctx, cancel := context.WithTimeout(context.Background(), oidcTimeout*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", g.reqURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+g.reqToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer func() {
		err := resp.Body.Close()
		if err != nil {
			g.output.MsgStdErrV("failed to close response body: %v", err)
		}
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	type response struct {
		Value string `json:"value"`
	}

	var tokresp response
	err = stdjson.Unmarshal(data, &tokresp)
	if err != nil {
		return err
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	g.token = tokresp.Value
	g.jwtClaims, err = tokenClaims(g.token)
	if err != nil {
		return err
	}
	exp, ok := g.jwtClaims["exp"].(float64)
	if !ok {
		return errors.E(`cached JWT token has no "exp" field`)
	}
	sec, dec := math.Modf(exp)
	g.expireAt = time.Unix(int64(sec), int64(dec*(1e9)))

	repoOwner, ok := g.jwtClaims["repository_owner"].(string)
	if !ok {
		return errors.E(`GitHub OIDC JWT with no "repository_owner" payload field.`)
	}
	repoName, ok := g.jwtClaims["repository"].(string)
	if !ok {
		return errors.E(`GitHub OIDC JWT with no "repository" payload field.`)
	}
	g.repoOwner = repoOwner
	g.repoName = repoName
	return nil
}

func (g *githubOIDC) Claims() jwt.MapClaims {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.jwtClaims
}

func (g *githubOIDC) DisplayClaims() []keyValue {
	return []keyValue{
		{
			key:   "owner",
			value: g.repoOwner,
		},
		{
			key:   "repository",
			value: g.repoName,
		},
	}
}

func (g *githubOIDC) Token() (string, error) {
	if g.IsExpired() {
		err := g.Refresh()
		if err != nil {
			return "", err
		}
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.token, nil
}

func (g *githubOIDC) Info(cloudcfg cloudConfig) error {
	client := cloud.Client{
		BaseURL:    cloudcfg.baseAPI,
		Credential: g,
	}

	const apiTimeout = 5 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
	defer cancel()
	orgs, err := client.MemberOrganizations(ctx)
	if err != nil {
		return err
	}

	if len(orgs) > 0 {
		cloudcfg.output.MsgStdOut("status: signed in")
	} else {
		cloudcfg.output.MsgStdOut("status: untrusted")
	}

	cloudcfg.output.MsgStdOut("provider: %s", g.Name())

	for _, kv := range g.DisplayClaims() {
		cloudcfg.output.MsgStdOut("%s: %s", kv.key, kv.value)
	}

	if len(orgs) > 0 {
		cloudcfg.output.MsgStdOut("organizations: %s", orgs)
	}

	if len(orgs) == 0 {
		cloudcfg.output.MsgStdErr("Warning: You are not part of an organization. Please visit cloud.terramate.io to create an organization.")
	}
	return nil
}
