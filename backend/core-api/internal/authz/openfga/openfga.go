// Package openfga is the OpenFGA-backed authz.Provider: real relationship
// tuples and Check calls against the running OpenFGA server, not a fake.
//
// OpenFGA's store is looked up by name and created (with its authorization
// model) on first sight, self-healing across restarts of an OpenFGA
// instance running with the ephemeral in-memory datastore (matching this
// repo's local dev config) the same way Keycloak's realm import does.
package openfga

import (
	"context"
	"errors"
	"fmt"

	fgaSdk "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"

	"github.com/example/core-platform/backend/core-api/internal/authz"
)

const storeName = "core-platform"

type Config struct {
	APIURL string
}

type Provider struct {
	client *client.OpenFgaClient
}

func New(ctx context.Context, cfg Config) (*Provider, error) {
	fgaClient, err := client.NewSdkClient(&client.ClientConfiguration{ApiUrl: cfg.APIURL})
	if err != nil {
		return nil, fmt.Errorf("new openfga client: %w", err)
	}

	storeID, justCreated, err := findOrCreateStore(ctx, fgaClient)
	if err != nil {
		return nil, err
	}
	if err := fgaClient.SetStoreId(storeID); err != nil {
		return nil, fmt.Errorf("set openfga store id: %w", err)
	}

	if justCreated {
		if err := writeModel(ctx, fgaClient); err != nil {
			return nil, err
		}
	}

	return &Provider{client: fgaClient}, nil
}

func findOrCreateStore(ctx context.Context, c *client.OpenFgaClient) (id string, justCreated bool, err error) {
	listed, err := c.ListStores(ctx).Execute()
	if err != nil {
		return "", false, fmt.Errorf("list openfga stores: %w", err)
	}
	for _, s := range listed.Stores {
		if s.Name == storeName {
			return s.Id, false, nil
		}
	}
	created, err := c.CreateStore(ctx).Body(client.ClientCreateStoreRequest{Name: storeName}).Execute()
	if err != nil {
		return "", false, fmt.Errorf("create openfga store: %w", err)
	}
	return created.Id, true, nil
}

// writeModel defines the platform-wide admin relation platform.admin
// grants map onto. Resource-specific relationship types (e.g. one profile
// visible to another via a real relationship) are added here as later
// phases (Relationships, Phase 8) introduce concrete relationship data to
// model - not invented speculatively now.
func writeModel(ctx context.Context, c *client.OpenFgaClient) error {
	userType := []fgaSdk.RelationReference{{Type: "user"}}
	_, err := c.WriteAuthorizationModel(ctx).Body(fgaSdk.WriteAuthorizationModelRequest{
		SchemaVersion: "1.1",
		TypeDefinitions: []fgaSdk.TypeDefinition{
			{Type: "user"},
			{
				Type: "platform",
				Relations: &map[string]fgaSdk.Userset{
					"admin": {This: &map[string]interface{}{}},
				},
				Metadata: &fgaSdk.Metadata{
					Relations: &map[string]fgaSdk.RelationMetadata{
						"admin": {DirectlyRelatedUserTypes: &userType},
					},
				},
			},
		},
	}).Execute()
	if err != nil {
		return fmt.Errorf("write openfga authorization model: %w", err)
	}
	return nil
}

func (p *Provider) Can(ctx context.Context, subjectUserID string, action authz.Action, resource authz.Resource) (bool, error) {
	resp, err := p.client.Check(ctx).Body(client.ClientCheckRequest{
		User:     "user:" + subjectUserID,
		Relation: string(action),
		Object:   resource.Type + ":" + resource.ID,
	}).Execute()
	if err != nil {
		return false, fmt.Errorf("openfga check: %w", err)
	}
	if resp.Allowed == nil {
		return false, nil
	}
	return *resp.Allowed, nil
}

// Grant is idempotent: writing a tuple that already exists is a no-op, not
// an error, matching RoleRepository.AssignRole's ON CONFLICT DO NOTHING -
// callers (like authz.Service.AssignRole) rely on being able to call this
// more than once for the same grant without it failing. The SDK's
// Conflict.OnDuplicateWrites option exists for exactly this but isn't
// honored by the OpenFGA server version this repo runs locally (confirmed
// live: the server still 400s), so it's handled by inspecting the error
// instead.
func (p *Provider) Grant(ctx context.Context, subjectUserID string, action authz.Action, resource authz.Resource) error {
	_, err := p.client.Write(ctx).Body(client.ClientWriteRequest{
		Writes: []client.ClientTupleKey{
			{User: "user:" + subjectUserID, Relation: string(action), Object: resource.Type + ":" + resource.ID},
		},
	}).Execute()
	if err != nil && !isWriteFailedDueToInvalidInput(err) {
		return fmt.Errorf("openfga grant: %w", err)
	}
	return nil
}

// Revoke is idempotent for the symmetric reason Grant is: deleting a tuple
// that doesn't exist (already revoked, or never granted) is a no-op.
func (p *Provider) Revoke(ctx context.Context, subjectUserID string, action authz.Action, resource authz.Resource) error {
	_, err := p.client.Write(ctx).Body(client.ClientWriteRequest{
		Deletes: []client.ClientTupleKeyWithoutCondition{
			{User: "user:" + subjectUserID, Relation: string(action), Object: resource.Type + ":" + resource.ID},
		},
	}).Execute()
	if err != nil && !isWriteFailedDueToInvalidInput(err) {
		return fmt.Errorf("openfga revoke: %w", err)
	}
	return nil
}

// isWriteFailedDueToInvalidInput matches the one OpenFGA validation error
// that, in context, always means "this write/delete was already a no-op" -
// Grant only ever writes and Revoke only ever deletes, so there's no
// ambiguity between "tuple already exists" and "tuple doesn't exist"
// despite them sharing this error code.
func isWriteFailedDueToInvalidInput(err error) bool {
	var validationErr fgaSdk.FgaApiValidationError
	return errors.As(err, &validationErr) && validationErr.ResponseCode() == fgaSdk.ERRORCODE_WRITE_FAILED_DUE_TO_INVALID_INPUT
}
