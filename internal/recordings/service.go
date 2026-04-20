package recordings

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository interface {
	List(ctx context.Context, limit, offset int) ([]Recording, error)
	GetByID(ctx context.Context, id string) (*Recording, error)
	ExistsByObjectKey(ctx context.Context, objectKey string) (bool, error)
	Create(ctx context.Context, rec *Recording) error
}

type ObjectStorage interface {
	UploadFile(ctx context.Context, objectKey, absolutePath string) (string, error)
}

type Service struct {
	repo               Repository
	objectStorage      ObjectStorage
	recordingsDir      string
	stableWindow       time.Duration
	maxUploadFileBytes int64
}

func NewService(repo Repository, objectStorage ObjectStorage, recordingsDir string, stableWindow time.Duration, maxUploadFileBytes int64) *Service {
	return &Service{
		repo:               repo,
		objectStorage:      objectStorage,
		recordingsDir:      recordingsDir,
		stableWindow:       stableWindow,
		maxUploadFileBytes: maxUploadFileBytes,
	}
}

func (s *Service) List(ctx context.Context, limit, offset int) ([]Recording, error) {
	return s.repo.List(ctx, limit, offset)
}

func (s *Service) GetByID(ctx context.Context, id string) (*Recording, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) Reconcile(ctx context.Context) (int, error) {
	paths, err := s.collectCandidateFiles()
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, path := range paths {
		ok, err := s.tryProcessFile(ctx, path)
		if err != nil {
			log.Printf("reconcile: failed processing %s: %v", path, err)
			continue
		}
		if ok {
			processed++
		}
	}
	return processed, nil
}

func (s *Service) collectCandidateFiles() ([]string, error) {
	var files []string
	err := filepath.WalkDir(s.recordingsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext == ".mp4" || ext == ".ts" || ext == ".mkv" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func (s *Service) tryProcessFile(ctx context.Context, absolutePath string) (bool, error) {
	info, err := os.Stat(absolutePath)
	if err != nil {
		return false, err
	}
	if info.Size() == 0 {
		return false, nil
	}
	if info.Size() > s.maxUploadFileBytes {
		return false, fmt.Errorf("file too large: %d", info.Size())
	}
	if time.Since(info.ModTime()) < s.stableWindow {
		return false, nil
	}

	rel, err := filepath.Rel(s.recordingsDir, absolutePath)
	if err != nil {
		return false, err
	}
	objectKey := filepath.ToSlash(rel)
	exists, err := s.repo.ExistsByObjectKey(ctx, objectKey)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}

	playbackURL, err := s.objectStorage.UploadFile(ctx, objectKey, absolutePath)
	if err != nil {
		return false, err
	}

	rec := &Recording{
		ID:             uuid.NewString(),
		StreamKey:      deriveStreamKey(objectKey),
		Title:          deriveTitle(objectKey),
		Description:    "Grabacion procesada automaticamente desde MediaMTX.",
		InstructorName: "Profesor",
		StartedAt:      info.ModTime(),
		EndedAt:        info.ModTime(),
		DurationSec:    0,
		ObjectKey:      objectKey,
		PlaybackURL:    playbackURL,
		Status:         StatusReady,
	}
	if err := s.repo.Create(ctx, rec); err != nil {
		return false, err
	}
	return true, nil
}

func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func deriveStreamKey(objectKey string) string {
	parts := strings.Split(objectKey, "/")
	if len(parts) > 1 && parts[0] != "" {
		return parts[0]
	}
	return "unknown"
}

func deriveTitle(objectKey string) string {
	base := filepath.Base(objectKey)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
