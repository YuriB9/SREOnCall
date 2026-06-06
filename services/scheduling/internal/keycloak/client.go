package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sre-oncall/scheduling/internal/domain"
)

// Client calls the Keycloak Admin REST API using client credentials.
type Client struct {
	adminURL     string // e.g. http://keycloak:8080
	realm        string
	clientID     string
	clientSecret string
	httpClient   *http.Client
}

func New(adminURL, realm, clientID, clientSecret string) *Client {
	return &Client{
		adminURL:     adminURL,
		realm:        realm,
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

// GetMembers returns members of the group named `slug` and its `admins` subgroup.
// Members of /slug/admins are returned with role "admin"; others with role "member".
func (c *Client) GetMembers(ctx context.Context, slug string) ([]domain.Member, error) {
	token, err := c.clientCredentials(ctx)
	if err != nil {
		return nil, fmt.Errorf("keycloak: get token: %w", err)
	}

	// Find the group by path.
	groupID, err := c.findGroupByPath(ctx, token, "/"+slug)
	if err != nil {
		return nil, fmt.Errorf("keycloak: find group %q: %w", slug, err)
	}

	// Fetch members of the main group.
	members, err := c.groupMembers(ctx, token, groupID)
	if err != nil {
		return nil, fmt.Errorf("keycloak: group members: %w", err)
	}

	// Find admins subgroup.
	adminGroupID, err := c.findSubgroup(ctx, token, groupID, "admins")
	if err == nil && adminGroupID != "" {
		admins, err := c.groupMembers(ctx, token, adminGroupID)
		if err != nil {
			return nil, fmt.Errorf("keycloak: admin group members: %w", err)
		}
		adminSet := make(map[string]struct{}, len(admins))
		for _, a := range admins {
			adminSet[a.UserID] = struct{}{}
		}
		for i := range members {
			if _, isAdmin := adminSet[members[i].UserID]; isAdmin {
				members[i].Role = "admin"
			}
		}
	}

	return members, nil
}

// clientCredentials returns an admin access token via client_credentials grant.
func (c *Client) clientCredentials(ctx context.Context) (string, error) {
	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", c.adminURL, c.realm)
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint %d: %s", resp.StatusCode, body)
	}
	var res struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return "", err
	}
	return res.AccessToken, nil
}

type kcGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

func (c *Client) findGroupByPath(ctx context.Context, token, path string) (string, error) {
	// Search by exact path — Keycloak /groups?search= does prefix match; filter locally.
	apiURL := fmt.Sprintf("%s/admin/realms/%s/groups?search=%s&exact=true",
		c.adminURL, c.realm, url.QueryEscape(strings.TrimPrefix(path, "/")))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var groups []kcGroup
	if err := json.NewDecoder(resp.Body).Decode(&groups); err != nil {
		return "", err
	}
	for _, g := range groups {
		if g.Path == path {
			return g.ID, nil
		}
	}
	return "", fmt.Errorf("group %q not found", path)
}

type kcUser struct {
	ID                string `json:"id"`
	PreferredUsername string `json:"username"`
}

func (c *Client) groupMembers(ctx context.Context, token, groupID string) ([]domain.Member, error) {
	apiURL := fmt.Sprintf("%s/admin/realms/%s/groups/%s/members?max=500",
		c.adminURL, c.realm, groupID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var users []kcUser
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, err
	}
	members := make([]domain.Member, len(users))
	for i, u := range users {
		members[i] = domain.Member{
			UserID:            u.ID,
			PreferredUsername: u.PreferredUsername,
			Role:              "member",
		}
	}
	return members, nil
}

func (c *Client) findSubgroup(ctx context.Context, token, parentID, name string) (string, error) {
	apiURL := fmt.Sprintf("%s/admin/realms/%s/groups/%s/children",
		c.adminURL, c.realm, parentID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var groups []kcGroup
	if err := json.NewDecoder(resp.Body).Decode(&groups); err != nil {
		return "", err
	}
	for _, g := range groups {
		if g.Name == name {
			return g.ID, nil
		}
	}
	return "", nil
}
