package coresdk

import "context"

// Typed convenience methods for a representative slice of the real API
// - "core identity" (platform, identity, users, devices, applications) -
// rather than all 146 registered routes. Every other route is still
// reachable via Client.Do with the same auth/retry/correlation handling;
// hand-typing wrappers for the rest is real, additive work anyone can
// do the same way these were written (copy a *http.go response struct's
// JSON shape verbatim - never guessed), not a framework limitation.

// Platform is GET /platform's response.
type Platform struct {
	Name        string `json:"name"`
	Environment string `json:"environment"`
	APIVersion  string `json:"apiVersion"`
	// Version is the real short git commit SHA core-api was built from
	// (Phase 29) - "dev" outside a git checkout or when git isn't on PATH.
	Version string `json:"version"`
}

func (c *Client) GetPlatform(ctx context.Context) (Platform, error) {
	var out Platform
	err := c.Do(ctx, "GET", "/v1/platform", nil, &out)
	return out, err
}

// Identity is GET /identity/me's response - the resolved token identity,
// distinct from User (see IdentityMe vs UsersMe).
type Identity struct {
	ID              string `json:"id"`
	Provider        string `json:"provider"`
	ProviderSubject string `json:"providerSubject"`
	Status          string `json:"status"`
	UserID          string `json:"userId"`
}

func (c *Client) IdentityMe(ctx context.Context) (Identity, error) {
	var out Identity
	err := c.Do(ctx, "GET", "/v1/identity/me", nil, &out)
	return out, err
}

// User mirrors internal/users/http.go's userResponse.
type User struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	AvatarRef   string `json:"avatarRef,omitempty"`
	Locale      string `json:"locale"`
	Timezone    string `json:"timezone"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// UpdateUserInput is the request body for UsersUpdateMe - at least one
// field is required, matching the server's own validation.
type UpdateUserInput struct {
	DisplayName string `json:"displayName,omitempty"`
	AvatarRef   string `json:"avatarRef,omitempty"`
	Locale      string `json:"locale,omitempty"`
	Timezone    string `json:"timezone,omitempty"`
	Status      string `json:"status,omitempty"`
}

func (c *Client) UsersMe(ctx context.Context) (User, error) {
	var out User
	err := c.Do(ctx, "GET", "/v1/users/me", nil, &out)
	return out, err
}

func (c *Client) UsersUpdateMe(ctx context.Context, in UpdateUserInput) (User, error) {
	var out User
	err := c.Do(ctx, "PATCH", "/v1/users/me", in, &out)
	return out, err
}

func (c *Client) UsersGet(ctx context.Context, id string) (User, error) {
	var out User
	err := c.Do(ctx, "GET", "/v1/users/"+id, nil, &out)
	return out, err
}

// UsersList is platform.admin-only (Phase 25's admin-wide listing).
func (c *Client) UsersList(ctx context.Context, cursor string) (Page[User], error) {
	path := "/v1/users"
	if cursor != "" {
		path += "?cursor=" + cursor
	}
	var out Page[User]
	err := c.Do(ctx, "GET", path, nil, &out)
	return out, err
}

// Device mirrors internal/devices/http.go's deviceResponse. The raw
// push token is write-only on the server and never appears here - see
// HasPushToken.
type Device struct {
	ID             string `json:"id"`
	ClientDeviceID string `json:"clientDeviceId"`
	Platform       string `json:"platform"`
	OSVersion      string `json:"osVersion,omitempty"`
	AppVersion     string `json:"appVersion,omitempty"`
	Locale         string `json:"locale"`
	Timezone       string `json:"timezone"`
	HasPushToken   bool   `json:"hasPushToken"`
	SessionStatus  string `json:"sessionStatus"`
	LastActiveAt   string `json:"lastActiveAt"`
	CreatedAt      string `json:"createdAt"`
}

// RegisterDeviceInput is the request body for DevicesRegister - the
// roadmap names "device registration" as its own SDK responsibility,
// distinct from generic "API calls," since every realtime connection
// (see RealtimeClient.Dial) needs a registered device id first.
type RegisterDeviceInput struct {
	ClientDeviceID string `json:"clientDeviceId"`
	Platform       string `json:"platform"`
	OSVersion      string `json:"osVersion,omitempty"`
	AppVersion     string `json:"appVersion,omitempty"`
	Locale         string `json:"locale,omitempty"`
	Timezone       string `json:"timezone,omitempty"`
	PushToken      string `json:"pushToken,omitempty"`
}

func (c *Client) DevicesRegister(ctx context.Context, in RegisterDeviceInput) (Device, error) {
	var out Device
	err := c.Do(ctx, "POST", "/v1/users/me/devices", in, &out)
	return out, err
}

func (c *Client) DevicesList(ctx context.Context) ([]Device, error) {
	var out struct {
		Items []Device `json:"items"`
	}
	err := c.Do(ctx, "GET", "/v1/users/me/devices", nil, &out)
	return out.Items, err
}

func (c *Client) DevicesRevoke(ctx context.Context, id string) error {
	return c.Do(ctx, "DELETE", "/v1/users/me/devices/"+id, nil, nil)
}

// Application mirrors internal/applications/http.go's applicationResponse.
type Application struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type CreateApplicationInput struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type UpdateApplicationInput struct {
	Name   string `json:"name,omitempty"`
	Status string `json:"status,omitempty"`
}

func (c *Client) AppsCreate(ctx context.Context, in CreateApplicationInput) (Application, error) {
	var out Application
	err := c.Do(ctx, "POST", "/v1/apps", in, &out)
	return out, err
}

func (c *Client) AppsList(ctx context.Context, cursor string) (Page[Application], error) {
	path := "/v1/apps"
	if cursor != "" {
		path += "?cursor=" + cursor
	}
	var out Page[Application]
	err := c.Do(ctx, "GET", path, nil, &out)
	return out, err
}

func (c *Client) AppsGet(ctx context.Context, id string) (Application, error) {
	var out Application
	err := c.Do(ctx, "GET", "/v1/apps/"+id, nil, &out)
	return out, err
}

func (c *Client) AppsUpdate(ctx context.Context, id string, in UpdateApplicationInput) (Application, error) {
	var out Application
	err := c.Do(ctx, "PATCH", "/v1/apps/"+id, in, &out)
	return out, err
}
