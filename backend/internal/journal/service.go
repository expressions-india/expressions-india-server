package journal

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dhruvpurohit2k/expressions-india-backend/internal/dto"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/models"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/pkg/utils"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/storage"
	"gorm.io/gorm"
)

type Service struct {
	db *gorm.DB
	s3 *storage.S3
}

func (s *Service) GetAllJournals() ([]models.Journal, error) {
	journals := []models.Journal{}
	err := s.db.Preload("Chapters").Preload("Chapters.Authors").Preload("Media").Preload("Chapters.Media").Find(&journals).Error
	return journals, err
}

func (s *Service) Get() ([]dto.JournalListItemDTO, error) {
	journals := []models.Journal{}
	if err := s.db.Find(&journals).Error; err != nil {
		return nil, err
	}
	var journaldtos []dto.JournalListItemDTO
	for _, journal := range journals {
		journaldtos = append(journaldtos, dto.JournalListItemDTO{
			ID:         journal.ID,
			Title:      journal.Title,
			Volume:     journal.Volume,
			Issue:      journal.Issue,
			StartMonth: journal.StartMonth,
			EndMonth:   journal.EndMonth,
			Year:       journal.Year,
		})
	}
	return journaldtos, nil
}

func (s *Service) GetJournalListFiltered(filter utils.JournalFilter) ([]dto.JournalListItemDTO, int64, error) {
	var journals []models.Journal
	var total int64

	base := s.db.Model(&models.Journal{})
	if filter.Search != "" {
		base = base.Where("LOWER(title) LIKE LOWER(?)", "%"+filter.Search+"%")
	}
	if filter.Year > 0 {
		base = base.Where("year = ?", filter.Year)
	}

	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := base.Order("year DESC, issue DESC").Limit(filter.Limit).Offset(filter.Offset).Find(&journals).Error; err != nil {
		return nil, 0, err
	}

	result := make([]dto.JournalListItemDTO, 0, len(journals))
	for _, journal := range journals {
		result = append(result, dto.JournalListItemDTO{
			ID:         journal.ID,
			Title:      journal.Title,
			Volume:     journal.Volume,
			Issue:      journal.Issue,
			StartMonth: journal.StartMonth,
			EndMonth:   journal.EndMonth,
			Year:       journal.Year,
		})
	}
	return result, total, nil
}

func (s *Service) GetJournalById(id string) (models.Journal, error) {
	journal := models.Journal{}
	if err := s.db.Where("id = ?", id).Preload("Chapters").Preload("Chapters.Authors").Preload("Media").Preload("Chapters.Media").First(&journal).Error; err != nil {
		return models.Journal{}, err
	}
	return journal, nil
}

func (s *Service) DeleteJournal(id string) error {
	var journal models.Journal
	if err := s.db.Preload("Media").Preload("Chapters").Preload("Chapters.Media").Preload("Chapters.Authors").First(&journal, "id = ?", id).Error; err != nil {
		return err
	}

	for _, chapter := range journal.Chapters {
		if err := s.db.Model(&chapter).Association("Authors").Clear(); err != nil {
			return err
		}
		if chapter.MediaId != nil {
			if err := s.db.Delete(&models.Media{}, "id = ?", *chapter.MediaId).Error; err != nil {
				return err
			}
			if err := s.s3.Delete(*chapter.MediaId); err != nil {
				return err
			}
		}
		if err := s.db.Delete(&chapter).Error; err != nil {
			return err
		}
	}

	if journal.MediaId != nil {
		if err := s.db.Delete(&models.Media{}, "id = ?", *journal.MediaId).Error; err != nil {
			return err
		}
		if err := s.s3.Delete(*journal.MediaId); err != nil {
			return err
		}
	}

	return s.db.Delete(&journal).Error
}

func NewService(db *gorm.DB, s3 *storage.S3) *Service {
	return &Service{db: db, s3: s3}
}

// CreateJournal creates a new journal using pre-uploaded refs.
func (s *Service) CreateJournal(req *dto.JournalCreateRequestDTO) error {
	chapters, err := dto.ParseJournalChapters(req.ChaptersJSON)
	if err != nil {
		return fmt.Errorf("invalid chapters JSON: %w", err)
	}
	cover, err := parseMediaRefField(req.CoverMediaUpload)
	if err != nil {
		return fmt.Errorf("invalid coverMediaUpload: %w", err)
	}

	desc := req.Description
	journal := models.Journal{
		Title:       req.Title,
		Description: nilIfEmpty(&desc),
		Volume:      req.Volume,
		Issue:       req.Issue,
		StartMonth:  req.StartMonth,
		EndMonth:    req.EndMonth,
		Year:        req.Year,
	}
	if cover != nil {
		media := s.mediaFromRef(*cover)
		if err := s.db.Create(&media).Error; err != nil {
			return err
		}
		journal.MediaId = &media.ID
	}
	if err := s.db.Create(&journal).Error; err != nil {
		return err
	}

	for _, ch := range chapters {
		if err := s.createChapter(journal.ID, ch); err != nil {
			return err
		}
	}
	return nil
}

// UpdateJournal updates a journal using pre-uploaded refs.
func (s *Service) UpdateJournal(id string, req *dto.JournalUpdateRequestDTO) error {
	chapters, err := dto.ParseJournalChapters(req.ChaptersJSON)
	if err != nil {
		return fmt.Errorf("invalid chapters JSON: %w", err)
	}
	deletedChapterIDs, err := dto.ParseDeletedChapterIDs(req.DeletedChapterIDsJSON)
	if err != nil {
		return fmt.Errorf("invalid deletedChapterIds: %w", err)
	}
	cover, err := parseMediaRefField(req.CoverMediaUpload)
	if err != nil {
		return fmt.Errorf("invalid coverMediaUpload: %w", err)
	}

	var journal models.Journal
	if err := s.db.Preload("Chapters").Preload("Chapters.Authors").First(&journal, "id = ?", id).Error; err != nil {
		return err
	}

	desc := req.Description
	journal.Title = req.Title
	journal.Description = nilIfEmpty(&desc)
	journal.Volume = req.Volume
	journal.Issue = req.Issue
	journal.StartMonth = req.StartMonth
	journal.EndMonth = req.EndMonth
	journal.Year = req.Year

	// Cover: drop if marked deleted or replaced; attach new if provided.
	if req.DeletedCoverMediaID != "" && journal.MediaId != nil && *journal.MediaId == req.DeletedCoverMediaID {
		if err := s.deleteMedia(*journal.MediaId); err != nil {
			return err
		}
		journal.MediaId = nil
	}
	if cover != nil {
		if journal.MediaId != nil {
			if err := s.deleteMedia(*journal.MediaId); err != nil {
				return err
			}
		}
		media := s.mediaFromRef(*cover)
		if err := s.db.Create(&media).Error; err != nil {
			return err
		}
		journal.MediaId = &media.ID
	}

	if err := s.db.Save(&journal).Error; err != nil {
		return err
	}

	for _, chID := range deletedChapterIDs {
		var ch models.JournalChapter
		if err := s.db.First(&ch, "id = ?", chID).Error; err != nil {
			continue
		}
		if err := s.db.Model(&ch).Association("Authors").Clear(); err != nil {
			return err
		}
		if ch.MediaId != nil {
			if err := s.deleteMedia(*ch.MediaId); err != nil {
				return err
			}
		}
		if err := s.db.Delete(&ch).Error; err != nil {
			return err
		}
	}

	for _, chInput := range chapters {
		if chInput.ID == "" {
			if err := s.createChapter(journal.ID, chInput); err != nil {
				return err
			}
			continue
		}
		var chapter models.JournalChapter
		if err := s.db.Preload("Authors").First(&chapter, "id = ?", chInput.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return err
		}
		chDesc := chInput.Description
		chapter.Title = chInput.Title
		chapter.Description = nilIfEmpty(&chDesc)

		if chInput.DeletedMediaID != "" && chapter.MediaId != nil && *chapter.MediaId == chInput.DeletedMediaID {
			if err := s.deleteMedia(*chapter.MediaId); err != nil {
				return err
			}
			chapter.MediaId = nil
		}
		if chInput.MediaUpload != nil {
			if chapter.MediaId != nil {
				if err := s.deleteMedia(*chapter.MediaId); err != nil {
					return err
				}
			}
			media := s.mediaFromRef(*chInput.MediaUpload)
			if err := s.db.Create(&media).Error; err != nil {
				return err
			}
			chapter.MediaId = &media.ID
		}

		authors, err := s.upsertAuthors(chInput.Authors)
		if err != nil {
			return err
		}
		if err := s.db.Model(&chapter).Association("Authors").Replace(authors); err != nil {
			return err
		}
		if err := s.db.Save(&chapter).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) createChapter(journalID string, ch dto.JournalChapterInput) error {
	chDesc := ch.Description
	chapter := models.JournalChapter{
		JournalId:   journalID,
		Title:       ch.Title,
		Description: nilIfEmpty(&chDesc),
	}
	if ch.MediaUpload != nil {
		media := s.mediaFromRef(*ch.MediaUpload)
		if err := s.db.Create(&media).Error; err != nil {
			return err
		}
		chapter.MediaId = &media.ID
	}
	authors, err := s.upsertAuthors(ch.Authors)
	if err != nil {
		return err
	}
	chapter.Authors = authors
	return s.db.Create(&chapter).Error
}

func (s *Service) mediaFromRef(ref dto.UploadedMediaRef) models.Media {
	return models.Media{
		ID:       ref.ID,
		Name:     ref.Name,
		URL:      s.s3.PublicURL(ref.ID),
		FileType: ref.FileType,
	}
}

func parseMediaRefField(raw string) (*dto.UploadedMediaRef, error) {
	if raw == "" {
		return nil, nil
	}
	var ref dto.UploadedMediaRef
	if err := json.Unmarshal([]byte(raw), &ref); err != nil {
		return nil, err
	}
	return &ref, nil
}

func (s *Service) deleteMedia(mediaID string) error {
	if err := s.db.Delete(&models.Media{}, "id = ?", mediaID).Error; err != nil {
		return err
	}
	return s.s3.Delete(mediaID)
}

// upsertAuthors fetches all existing authors in one IN query, then batch-creates
// any that are missing. Eliminates the per-author N+1 query pattern.
func (s *Service) upsertAuthors(names []string) ([]models.Author, error) {
	// De-duplicate names, preserving order, and filter empty strings.
	seen := map[string]bool{}
	unique := make([]string, 0, len(names))
	for _, raw := range names {
		if raw != "" && !seen[raw] {
			seen[raw] = true
			unique = append(unique, raw)
		}
	}
	if len(unique) == 0 {
		return []models.Author{}, nil
	}

	// Single query to fetch all already-existing authors.
	var existing []models.Author
	if err := s.db.Where("name IN ?", unique).Find(&existing).Error; err != nil {
		return nil, err
	}

	existingByName := make(map[string]models.Author, len(existing))
	for _, a := range existing {
		existingByName[a.Name] = a
	}

	// Collect missing authors and create them individually (sqlite
	// does not support batch-insert with RETURNING, but each create is cheap).
	result := make([]models.Author, 0, len(unique))
	for _, name := range unique {
		if a, ok := existingByName[name]; ok {
			result = append(result, a)
		} else {
			newAuthor := models.Author{Name: name}
			if err := s.db.Create(&newAuthor).Error; err != nil {
				return nil, err
			}
			result = append(result, newAuthor)
		}
	}
	return result, nil
}

func nilIfEmpty(p *string) *string {
	if p == nil || *p == "" {
		return nil
	}
	return p
}
