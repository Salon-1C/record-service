package mysql

import (
	"context"

	"github.com/Salon-1C/record-service/internal/recordings"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func New(dsn string) (*Repository, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&recordings.Recording{}); err != nil {
		return nil, err
	}
	return &Repository{db: db}, nil
}

func (r *Repository) List(ctx context.Context, limit, offset int) ([]recordings.Recording, error) {
	var rows []recordings.Recording
	err := r.db.WithContext(ctx).
		Order("started_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&rows).Error
	return rows, err
}

func (r *Repository) GetByID(ctx context.Context, id string) (*recordings.Recording, error) {
	var row recordings.Recording
	err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) ExistsByObjectKey(ctx context.Context, objectKey string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&recordings.Recording{}).
		Where("object_key = ?", objectKey).
		Count(&count).Error
	return count > 0, err
}

func (r *Repository) Create(ctx context.Context, rec *recordings.Recording) error {
	return r.db.WithContext(ctx).Create(rec).Error
}
