package store

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/coderianx/Aither/internal/models"
)

// ErrNotFound is returned when the requested record does not exist.
var ErrNotFound = errors.New("record not found")

const providerColumns = `id, provider_name, api_key, base_url, created_at`

// CreateProvider inserts a new provider and returns its ID.
func CreateProvider(providerName, apiKey, baseURL string) (int64, error) {
	res, err := DB.Exec(
		`INSERT INTO providers (provider_name, api_key, base_url) VALUES (?, ?, ?)`,
		providerName, apiKey, baseURL,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to insert provider: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to read provider id: %w", err)
	}

	return id, nil
}

// ListProviders returns every configured provider, newest first.
func ListProviders() ([]models.Provider, error) {
	rows, err := DB.Query(
		`SELECT ` + providerColumns + ` FROM providers ORDER BY id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query providers: %w", err)
	}
	defer rows.Close()

	providers := make([]models.Provider, 0)

	for rows.Next() {
		var p models.Provider

		if err := rows.Scan(
			&p.ID,
			&p.ProviderName,
			&p.APIKey,
			&p.BaseURL,
			&p.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan provider: %w", err)
		}

		providers = append(providers, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate providers: %w", err)
	}

	return providers, nil
}

// GetProvider returns a single provider by its ID.
func GetProvider(id int64) (*models.Provider, error) {
	var p models.Provider

	err := DB.QueryRow(
		`SELECT `+providerColumns+` FROM providers WHERE id = ?`, id,
	).Scan(
		&p.ID,
		&p.ProviderName,
		&p.APIKey,
		&p.BaseURL,
		&p.CreatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query provider: %w", err)
	}

	return &p, nil
}

// DeleteProvider removes a provider by ID.
func DeleteProvider(id int64) error {
	res, err := DB.Exec(`DELETE FROM providers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete provider: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read delete result: %w", err)
	}

	if affected == 0 {
		return ErrNotFound
	}

	return nil
}
