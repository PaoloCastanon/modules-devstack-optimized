package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/repository"
	driver "github.com/go-sql-driver/mysql"
)

type Repository struct {
	db      *sql.DB
	timeout time.Duration
}

var _ repository.Repository = (*Repository)(nil)

func New(db *sql.DB, timeout time.Duration) *Repository { return &Repository{db: db, timeout: timeout} }

func (r *Repository) Create(ctx context.Context, image model.Image) error {
	if image.Status != "queued" {
		return errors.New("only queued metadata can be created")
	}
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin metadata transaction: %w", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO images
		(id, owner, name, status, visibility, disk_format, container_format, min_disk, min_ram, protected, os_hidden, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		image.ID, image.Owner, image.Name, image.Status, image.Visibility, image.DiskFormat, image.ContainerFormat,
		image.MinDisk, image.MinRAM, image.Protected, image.Hidden, image.CreatedAt.UTC(), image.UpdatedAt.UTC())
	if err != nil {
		var my *driver.MySQLError
		if errors.As(err, &my) && my.Number == 1062 {
			return repository.ErrDuplicate
		}
		return fmt.Errorf("insert image: %w", err)
	}
	keys := make([]string, 0, len(image.Properties))
	for key := range image.Properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, err = tx.ExecContext(ctx, "INSERT INTO image_properties (image_id, name, value) VALUES (?, ?, ?)", image.ID, key, image.Properties[key]); err != nil {
			return fmt.Errorf("insert image property: %w", err)
		}
	}
	for _, tag := range image.Tags {
		if _, err = tx.ExecContext(ctx, "INSERT INTO image_tags (image_id, value) VALUES (?, ?)", image.ID, tag); err != nil {
			return fmt.Errorf("insert image tag: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit metadata transaction: %w", err)
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, id string) (model.Image, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return model.Image{}, fmt.Errorf("begin metadata read: %w", err)
	}
	defer tx.Rollback()
	image := model.Image{Tags: []string{}, Properties: map[string]string{}}
	err = tx.QueryRowContext(ctx, `SELECT id, owner, name, status, visibility, disk_format, container_format,
		min_disk, min_ram, protected, os_hidden, size, virtual_size, checksum, os_hash_algo, os_hash_value, created_at, updated_at
		FROM images WHERE id=? AND deleted=FALSE`, id).Scan(
		&image.ID, &image.Owner, &image.Name, &image.Status, &image.Visibility, &image.DiskFormat, &image.ContainerFormat,
		&image.MinDisk, &image.MinRAM, &image.Protected, &image.Hidden, &image.Size, &image.VirtualSize, &image.Checksum, &image.HashAlgorithm, &image.HashValue, &image.CreatedAt, &image.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Image{}, repository.ErrNotFound
	}
	if err != nil {
		return model.Image{}, fmt.Errorf("read image: %w", err)
	}
	rows, err := tx.QueryContext(ctx, "SELECT name, value FROM image_properties WHERE image_id=? ORDER BY name", id)
	if err != nil {
		return model.Image{}, fmt.Errorf("read image properties: %w", err)
	}
	for rows.Next() {
		var key, value string
		if err = rows.Scan(&key, &value); err != nil {
			break
		}
		image.Properties[key] = value
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil {
		return model.Image{}, err
	}
	if rowErr != nil {
		return model.Image{}, rowErr
	}
	rows, err = tx.QueryContext(ctx, "SELECT value FROM image_tags WHERE image_id=? ORDER BY value", id)
	if err != nil {
		return model.Image{}, fmt.Errorf("read image tags: %w", err)
	}
	for rows.Next() {
		var tag string
		if err = rows.Scan(&tag); err != nil {
			break
		}
		image.Tags = append(image.Tags, tag)
	}
	rowErr = rows.Err()
	rows.Close()
	if err != nil {
		return model.Image{}, err
	}
	if rowErr != nil {
		return model.Image{}, rowErr
	}
	if err = tx.Commit(); err != nil {
		return model.Image{}, fmt.Errorf("complete metadata read: %w", err)
	}
	return image, nil
}
