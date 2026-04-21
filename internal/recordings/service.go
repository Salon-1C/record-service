package recordings

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
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
	UpsertStreamMetadata(ctx context.Context, md *StreamMetadata) error
	GetStreamMetadata(ctx context.Context, streamKey string) (*StreamMetadata, error)
}

type ObjectStorage interface {
	UploadFile(ctx context.Context, objectKey, absolutePath string) (string, error)
	UploadBytes(ctx context.Context, objectKey string, content []byte, contentType string) (string, error)
	OpenRead(ctx context.Context, objectKey string) (io.ReadCloser, string, int64, error)
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

func (s *Service) OpenPlaybackByID(ctx context.Context, id string) (*Recording, io.ReadCloser, string, int64, error) {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, "", 0, err
	}
	body, contentType, size, err := s.objectStorage.OpenRead(ctx, row.ObjectKey)
	if err != nil {
		return nil, nil, "", 0, err
	}
	return row, body, contentType, size, nil
}

func (s *Service) RegisterStreamMetadata(ctx context.Context, streamKey, title, description, instructorName string) error {
	streamKey = strings.TrimSpace(streamKey)
	if streamKey == "" {
		return fmt.Errorf("streamKey is required")
	}
	if strings.TrimSpace(title) == "" {
		title = "Transmision"
	}
	if strings.TrimSpace(instructorName) == "" {
		instructorName = "Profesor"
	}
	return s.repo.UpsertStreamMetadata(ctx, &StreamMetadata{
		StreamKey:      streamKey,
		Title:          title,
		Description:    description,
		InstructorName: instructorName,
	})
}

func (s *Service) ProcessQueuedSegment(ctx context.Context, streamPath, segmentPath, contentBase64 string) error {
	if streamPath == "" || segmentPath == "" || contentBase64 == "" {
		return fmt.Errorf("streamPath, segmentPath and contentBase64 are required")
	}
	data, err := base64.StdEncoding.DecodeString(contentBase64)
	if err != nil {
		return fmt.Errorf("invalid base64 payload: %w", err)
	}
	objectKey := filepath.ToSlash(strings.TrimPrefix(segmentPath, "/recordings/"))
	if objectKey == "" || objectKey == "." {
		objectKey = filepath.ToSlash(strings.TrimPrefix(segmentPath, "/"))
	}
	exists, err := s.repo.ExistsByObjectKey(ctx, objectKey)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	playbackURL, err := s.objectStorage.UploadBytes(ctx, objectKey, data, "video/mp4")
	if err != nil {
		return err
	}
	startedAt := deriveStartedAt(objectKey, time.Now().UTC())
	durationSec := int64(0)
	rec := &Recording{
		ID:             uuid.NewString(),
		StreamKey:      deriveStreamKey(objectKey),
		Title:          deriveTitle(objectKey),
		Description:    "Grabacion procesada asincronamente desde cola.",
		InstructorName: "Profesor",
		StartedAt:      startedAt,
		EndedAt:        startedAt,
		DurationSec:    durationSec,
		ObjectKey:      objectKey,
		PlaybackURL:    playbackURL,
		Status:         StatusReady,
	}
	if md, err := s.repo.GetStreamMetadata(ctx, rec.StreamKey); err == nil {
		rec.Title = md.Title
		rec.Description = md.Description
		rec.InstructorName = md.InstructorName
	}
	return s.repo.Create(ctx, rec)
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
	startedAt := deriveStartedAt(objectKey, info.ModTime())
	endedAt := info.ModTime()
	durationSec := int64(endedAt.Sub(startedAt).Seconds())
	if durationSec < 0 {
		durationSec = 0
	}

	rec := &Recording{
		ID:             uuid.NewString(),
		StreamKey:      deriveStreamKey(objectKey),
		Title:          deriveTitle(objectKey),
		Description:    "Grabacion procesada automaticamente desde MediaMTX.",
		InstructorName: "Profesor",
		StartedAt:      startedAt,
		EndedAt:        endedAt,
		DurationSec:    durationSec,
		ObjectKey:      objectKey,
		PlaybackURL:    playbackURL,
		Status:         StatusReady,
	}
	if md, err := s.repo.GetStreamMetadata(ctx, rec.StreamKey); err == nil {
		rec.Title = md.Title
		rec.Description = md.Description
		rec.InstructorName = md.InstructorName
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
	if len(parts) > 1 && parts[0] == "live" && parts[1] != "" {
		return parts[1]
	}
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return "unknown"
}

func deriveTitle(objectKey string) string {
	base := filepath.Base(objectKey)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func deriveStartedAt(objectKey string, fallback time.Time) time.Time {
	base := filepath.Base(objectKey)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	ts, err := time.ParseInLocation("2006-01-02_15-04-05", base, fallback.Location())
	if err != nil {
		return fallback
	}
	return ts
}
