// Copyright 2022 Harness Inc. All rights reserved.
// Use of this source code is governed by the Polyform Free Trial License
// that can be found in the LICENSE.md file for this repository.

package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/harness/gitness/internal/store"
	"github.com/harness/gitness/internal/store/database/dbtx"
	"github.com/harness/gitness/types"
	"github.com/harness/gitness/types/enum"

	"github.com/guregu/null"
	"github.com/jmoiron/sqlx"
)

var _ store.WebhookStore = (*WebhookStore)(nil)

// NewWebhookStore returns a new WebhookStore.
func NewWebhookStore(db *sqlx.DB) *WebhookStore {
	return &WebhookStore{
		db: db,
	}
}

// WebhookStore implements store.Webhook backed by a relational database.
type WebhookStore struct {
	db *sqlx.DB
}

// webhook is used to fetch webhook data from the database.
// The object should be later re-packed into a different struct to return it as an API response.
type webhook struct {
	ID        int64    `db:"webhook_id"`
	Version   int64    `db:"webhook_version"`
	RepoID    null.Int `db:"webhook_repo_id"`
	SpaceID   null.Int `db:"webhook_space_id"`
	CreatedBy int64    `db:"webhook_created_by"`
	Created   int64    `db:"webhook_created"`
	Updated   int64    `db:"webhook_updated"`

	URL      string `db:"webhook_url"`
	Secret   string `db:"webhook_secret"`
	Enabled  bool   `db:"webhook_enabled"`
	Insecure bool   `db:"webhook_insecure"`
	Triggers string `db:"webhook_triggers"`
}

const (
	webhookColumns = `
		 webhook_id
		,webhook_version
		,webhook_repo_id
		,webhook_space_id
		,webhook_created_by
		,webhook_created
		,webhook_updated
		,webhook_url
		,webhook_secret
		,webhook_enabled
		,webhook_insecure
		,webhook_triggers`

	webhookSelectBase = `
	SELECT` + webhookColumns + `
	FROM webhooks`
)

// Find finds the webhook by id.
func (s *WebhookStore) Find(ctx context.Context, id int64) (*types.Webhook, error) {
	const sqlQuery = webhookSelectBase + `
		WHERE webhook_id = $1`

	db := dbtx.GetAccessor(ctx, s.db)

	dst := &webhook{}
	if err := db.GetContext(ctx, dst, sqlQuery, id); err != nil {
		return nil, processSQLErrorf(err, "Select query failed")
	}

	res, err := mapToWebhook(dst)
	if err != nil {
		return nil, fmt.Errorf("failed to map webhook to external type: %w", err)
	}

	return res, nil
}

// Create creates a new webhook.
func (s *WebhookStore) Create(ctx context.Context, hook *types.Webhook) error {
	const sqlQuery = `
		INSERT INTO webhooks (
			webhook_repo_id
			,webhook_space_id
			,webhook_created_by
			,webhook_created
			,webhook_updated
			,webhook_url
			,webhook_secret
			,webhook_enabled
			,webhook_insecure
			,webhook_triggers
		) values (
			:webhook_repo_id
			,:webhook_space_id
			,:webhook_created_by
			,:webhook_created
			,:webhook_updated
			,:webhook_url
			,:webhook_secret
			,:webhook_enabled
			,:webhook_insecure
			,:webhook_triggers
		) RETURNING webhook_id`

	db := dbtx.GetAccessor(ctx, s.db)

	dbHook, err := mapToInternalWebhook(hook)
	if err != nil {
		return fmt.Errorf("failed to map webhook to internal db type: %w", err)
	}

	query, arg, err := db.BindNamed(sqlQuery, dbHook)
	if err != nil {
		return processSQLErrorf(err, "Failed to bind webhook object")
	}

	if err = db.QueryRowContext(ctx, query, arg...).Scan(&hook.ID); err != nil {
		return processSQLErrorf(err, "Insert query failed")
	}

	return nil
}

// Update updates an existing webhook.
func (s *WebhookStore) Update(ctx context.Context, hook *types.Webhook) error {
	const sqlQuery = `
		UPDATE webhooks
		SET
			webhook_version = :webhook_version
			,webhook_updated = :webhook_updated
			,webhook_url = :webhook_url
			,webhook_secret = :webhook_secret
			,webhook_enabled = :webhook_enabled
			,webhook_insecure = :webhook_insecure
			,webhook_triggers = :webhook_triggers
		WHERE webhook_id = :webhook_id and webhook_version = :webhook_version - 1`

	db := dbtx.GetAccessor(ctx, s.db)

	dbHook, err := mapToInternalWebhook(hook)
	if err != nil {
		return fmt.Errorf("failed to map webhook to internal db type: %w", err)
	}

	// update Version (used for optimistic locking) and Updated time
	dbHook.Version++
	dbHook.Updated = time.Now().UnixMilli()

	query, arg, err := db.BindNamed(sqlQuery, dbHook)
	if err != nil {
		return processSQLErrorf(err, "Failed to bind webhook object")
	}

	result, err := db.ExecContext(ctx, query, arg...)
	if err != nil {
		return processSQLErrorf(err, "failed to update webhook")
	}

	count, err := result.RowsAffected()
	if err != nil {
		return processSQLErrorf(err, "Failed to get number of updated rows")
	}

	if count == 0 {
		return store.ErrConflict
	}

	hook.Version = dbHook.Version
	hook.Updated = dbHook.Updated

	return nil
}

// Delete deletes the webhook for the given id.
func (s *WebhookStore) Delete(ctx context.Context, id int64) error {
	const sqlQuery = `
		DELETE FROM webhooks
		WHERE webhook_id = $1`

	if _, err := s.db.ExecContext(ctx, sqlQuery, id); err != nil {
		return processSQLErrorf(err, "The delete query failed")
	}

	return nil
}

// Count counts the webhooks for a given parent type and id.
func (s *WebhookStore) Count(ctx context.Context, parentType enum.WebhookParent, parentID int64,
	opts *types.WebhookFilter) (int64, error) {
	stmt := builder.
		Select("count(*)").
		From("webhooks")

	switch parentType {
	case enum.WebhookParentRepo:
		stmt = stmt.Where("webhook_repo_id = ?", parentID)
	case enum.WebhookParentSpace:
		stmt = stmt.Where("webhook_space_id = ?", parentID)
	default:
		return 0, fmt.Errorf("webhook parent type '%s' is not supported", parentType)
	}

	sql, args, err := stmt.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to convert query to sql: %w", err)
	}

	db := dbtx.GetAccessor(ctx, s.db)

	var count int64
	err = db.QueryRowContext(ctx, sql, args...).Scan(&count)
	if err != nil {
		return 0, processSQLErrorf(err, "Failed executing count query")
	}

	return count, nil
}

// List lists the webhooks for a given parent type and id.
func (s *WebhookStore) List(ctx context.Context, parentType enum.WebhookParent, parentID int64,
	opts *types.WebhookFilter) ([]*types.Webhook, error) {
	stmt := builder.
		Select(webhookColumns).
		From("webhooks")

	switch parentType {
	case enum.WebhookParentRepo:
		stmt = stmt.Where("webhook_repo_id = ?", parentID)
	case enum.WebhookParentSpace:
		stmt = stmt.Where("webhook_space_id = ?", parentID)
	default:
		return nil, fmt.Errorf("webhook parent type '%s' is not supported", parentType)
	}

	stmt = stmt.Limit(uint64(limit(opts.Size)))
	stmt = stmt.Offset(uint64(offset(opts.Page, opts.Size)))

	// fixed ordering by id (old ones first) - add customized ordering if deemed necessary
	stmt = stmt.OrderBy("webhook_id ASC")

	sql, args, err := stmt.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to convert query to sql: %w", err)
	}

	db := dbtx.GetAccessor(ctx, s.db)

	dst := []*webhook{}
	if err = db.SelectContext(ctx, &dst, sql, args...); err != nil {
		return nil, processSQLErrorf(err, "Select query failed")
	}

	res, err := mapToWebhooks(dst)
	if err != nil {
		return nil, fmt.Errorf("failed to map webhooks to external type: %w", err)
	}

	return res, nil
}

func mapToWebhook(hook *webhook) (*types.Webhook, error) {
	res := &types.Webhook{
		ID:        hook.ID,
		Version:   hook.Version,
		CreatedBy: hook.CreatedBy,
		Created:   hook.Created,
		Updated:   hook.Updated,
		URL:       hook.URL,
		Secret:    hook.Secret,
		Enabled:   hook.Enabled,
		Insecure:  hook.Insecure,
		Triggers:  triggersFromString(hook.Triggers),
	}

	switch {
	case hook.RepoID.Valid && hook.SpaceID.Valid:
		return nil, fmt.Errorf("both repoID and spaceID are set for hook %d", hook.ID)
	case hook.RepoID.Valid:
		res.ParentType = enum.WebhookParentRepo
		res.ParentID = hook.RepoID.Int64
	case hook.SpaceID.Valid:
		res.ParentType = enum.WebhookParentSpace
		res.ParentID = hook.SpaceID.Int64
	default:
		return nil, fmt.Errorf("neither repoID nor spaceID are set for hook %d", hook.ID)
	}

	return res, nil
}

func mapToInternalWebhook(hook *types.Webhook) (*webhook, error) {
	res := &webhook{
		ID:        hook.ID,
		Version:   hook.Version,
		CreatedBy: hook.CreatedBy,
		Created:   hook.Created,
		Updated:   hook.Updated,
		URL:       hook.URL,
		Secret:    hook.Secret,
		Enabled:   hook.Enabled,
		Insecure:  hook.Insecure,
		Triggers:  triggersToString(hook.Triggers),
	}

	switch hook.ParentType {
	case enum.WebhookParentRepo:
		res.RepoID = null.IntFrom(hook.ParentID)
	case enum.WebhookParentSpace:
		res.SpaceID = null.IntFrom(hook.ParentID)
	default:
		return nil, fmt.Errorf("webhook parent type '%s' is not supported", hook.ParentType)
	}

	return res, nil
}

func mapToWebhooks(hooks []*webhook) ([]*types.Webhook, error) {
	var err error
	m := make([]*types.Webhook, len(hooks))
	for i, hook := range hooks {
		m[i], err = mapToWebhook(hook)
		if err != nil {
			return nil, err
		}
	}
	return m, nil
}

// triggersSeparator defines the character that's used to join triggers for storing them in the DB
// ASSUMPTION: triggers are defined in an enum and don't contain ",".
const triggersSeparator = ","

func triggersFromString(triggersString string) []enum.WebhookTrigger {
	if triggersString == "" {
		return []enum.WebhookTrigger{}
	}

	rawTriggers := strings.Split(triggersString, triggersSeparator)

	triggers := make([]enum.WebhookTrigger, len(rawTriggers))
	for i, rawTrigger := range rawTriggers {
		// ASSUMPTION: trigger is valid value (as we wrote it to DB)
		triggers[i] = enum.WebhookTrigger(rawTrigger)
	}

	return triggers
}

func triggersToString(triggers []enum.WebhookTrigger) string {
	rawTriggers := make([]string, len(triggers))
	for i := range triggers {
		rawTriggers[i] = string(triggers[i])
	}

	return strings.Join(rawTriggers, triggersSeparator)
}
