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
	image, err := readImage(ctx, tx, id, false)
	if err != nil {
		return model.Image{}, err
	}
	if err = tx.Commit(); err != nil {
		return model.Image{}, fmt.Errorf("complete metadata read: %w", err)
	}
	return image, nil
}
