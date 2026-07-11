package event

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/dhruvpurohit2k/expressions-india-backend/internal/dto"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/models"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/pkg/utils"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/storage"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Service struct {
	db *gorm.DB
	s3 *storage.S3
}

func NewService(db *gorm.DB, s3 *storage.S3) *Service {
	return &Service{
		db: db,
		s3: s3,
	}
}

func (s *Service) GetUpcomingEvents(limit int, offset int) ([]dto.EventListItemDTO, int64, error) {
	var events []models.Event
	var total int64

	base := s.db.Model(&models.Event{}).
		Where("status = ?", "upcoming").
		Where("start_date >= ? OR start_date IS NULL", time.Now())

	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := base.Preload("Thumbnail").Order("COALESCE(start_date, end_date, created_at) ASC").Limit(limit).Offset(offset).Find(&events).Error; err != nil {
		return nil, 0, err
	}

	var result []dto.EventListItemDTO
	for _, event := range events {
		item := dto.EventListItemDTO{
			ID:        event.ID,
			Title:     event.Title,
			StartDate: event.StartDate,
			EndDate:   event.EndDate,
		}
		if event.Thumbnail != nil {
			item.ThumbnailURL = &event.Thumbnail.URL
		}
		result = append(result, item)
	}
	return result, total, nil
}

func (s *Service) GetPastEvents(limit int, offset int) ([]dto.EventListItemDTO, int64, error) {
	var events []models.Event
	var total int64

	base := s.db.Model(&models.Event{}).
		Where("status = ?", "completed")

	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := base.Preload("Thumbnail").Order("COALESCE(end_date, start_date, created_at) DESC").Limit(limit).Offset(offset).Find(&events).Error; err != nil {
		return nil, 0, err
	}

	var result []dto.EventListItemDTO
	for _, event := range events {
		item := dto.EventListItemDTO{
			ID:        event.ID,
			Title:     event.Title,
			StartDate: event.StartDate,
			EndDate:   event.EndDate,
		}
		if event.Thumbnail != nil {
			item.ThumbnailURL = &event.Thumbnail.URL
		}
		result = append(result, item)
	}
	return result, total, nil
}

func (s *Service) GetAllEvents() ([]models.Event, error) {
	var events []models.Event
	err := s.db.Preload("Thumbnail").
		Preload("Medias").
		Preload("Documents").
		Preload("VideoLinks").
		Preload("PromotionalMedia").
		Preload("PromotionalDocuments").
		Preload("Audiences").
		Find(&events).Error

	return events, err
}

func (s *Service) GetUpcomingEventsByAudience(audience string, limit int, offset int) ([]dto.EventListItemDTO, int64, error) {
	var events []models.Event
	var total int64

	base := s.db.Model(&models.Event{}).
		Where("status = ?", "upcoming").
		Where("start_date >= ? OR start_date IS NULL", time.Now()).
		Where(
			"events.id IN (SELECT ea.event_id FROM event_audience ea JOIN audiences a ON a.id = ea.audience_id WHERE a.name = ? OR a.name = 'all') OR events.id NOT IN (SELECT DISTINCT ea.event_id FROM event_audience ea)",
			audience,
		)

	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := base.Preload("Thumbnail").Order("COALESCE(start_date, end_date, created_at) ASC").Limit(limit).Offset(offset).Find(&events).Error; err != nil {
		return nil, 0, err
	}

	result := make([]dto.EventListItemDTO, 0, len(events))
	for _, e := range events {
		item := dto.EventListItemDTO{
			ID:        e.ID,
			Title:     e.Title,
			IsOnline:  e.IsOnline,
			IsPaid:    e.IsPaid,
			StartDate: e.StartDate,
			EndDate:   e.EndDate,
		}
		if e.Thumbnail != nil {
			item.ThumbnailURL = &e.Thumbnail.URL
		}
		result = append(result, item)
	}
	return result, total, nil
}

func (s *Service) GetEventById(id string) (*dto.EventDTO, error) {
	var event models.Event
	err := s.db.Where("id = ?", id).
		Preload("Thumbnail").
		Preload("Medias").
		Preload("Documents").
		Preload("VideoLinks").
		Preload("PromotionalVideoLinks").
		Preload("PromotionalMedia").
		Preload("PromotionalDocuments").
		Preload("Audiences").
		First(&event).Error

	audiences := make([]string, 0, len(event.Audiences))
	for _, audience := range event.Audiences {
		audiences = append(audiences, audience.Name)
	}
	result := &dto.EventDTO{
		Event:     event,
		Audiences: audiences,
	}

	return result, err
}

func (s *Service) CreateEvent(data *dto.EventCreateRequestDTO) error {
	var newEvent models.Event
	newEvent.Title = data.Title
	newEvent.Description = data.Description
	newEvent.Perks = datatypes.JSON(data.Perks)
	newEvent.StartDate = data.StartDate
	newEvent.EndDate = data.EndDate
	newEvent.StartTime = data.StartTime
	newEvent.EndTime = data.EndTime
	newEvent.Status = data.Status
	newEvent.Location = data.Location
	if data.IsOnline != nil {
		newEvent.IsOnline = *data.IsOnline
	}
	if data.IsPaid != nil {
		newEvent.IsPaid = *data.IsPaid
		if *data.IsPaid {
			if data.Price != nil {
				newEvent.Price = data.Price
			} else {
				return utils.NewValidationError("price is required for paid events")
			}
		} else {
			newEvent.Price = nil
		}
	}
	if data.RegistrationURL != nil {
		newEvent.RegistrationURL = *data.RegistrationURL
	}

	if data.ThumbnailUpload != "" {
		ref, err := parseMediaRef(data.ThumbnailUpload)
		if err != nil {
			return fmt.Errorf("invalid thumbnailUpload: %w", err)
		}
		media := s.mediaFromRef(ref)
		if err := s.db.Create(&media).Error; err != nil {
			return err
		}
		newEvent.ThumbnailID = &media.ID
		newEvent.Thumbnail = &media
	}

	if len(data.Audiences) > 0 {
		audienceRows, err := s.getAudience(data.Audiences)
		if err != nil {
			return err
		}
		newEvent.Audiences = audienceRows
	}

	if len(data.PromotionalMediaUploads) > 0 {
		refs, err := parseMediaRefs(data.PromotionalMediaUploads)
		if err != nil {
			return fmt.Errorf("invalid promotionalMediaUploads: %w", err)
		}
		newEvent.PromotionalMedia = s.mediasFromRefs(refs)
	}
	if len(data.PromotionalDocumentUploads) > 0 {
		refs, err := parseMediaRefs(data.PromotionalDocumentUploads)
		if err != nil {
			return fmt.Errorf("invalid promotionalDocumentUploads: %w", err)
		}
		newEvent.PromotionalDocuments = s.mediasFromRefs(refs)
	}
	if len(data.DocumentUploads) > 0 {
		refs, err := parseMediaRefs(data.DocumentUploads)
		if err != nil {
			return fmt.Errorf("invalid documentUploads: %w", err)
		}
		newEvent.Documents = s.mediasFromRefs(refs)
	}
	if len(data.MediaUploads) > 0 {
		refs, err := parseMediaRefs(data.MediaUploads)
		if err != nil {
			return fmt.Errorf("invalid mediaUploads: %w", err)
		}
		newEvent.Medias = s.mediasFromRefs(refs)
	}
	if len(data.VideoLinks) > 0 {
		videoLinks, err := s.getLink(data.VideoLinks)
		if err != nil {
			return err
		}
		newEvent.VideoLinks = videoLinks
	}
	if len(data.PromotionalVideoLinks) > 0 {
		promoLinks, err := s.getLink(data.PromotionalVideoLinks)
		if err != nil {
			return err
		}
		newEvent.PromotionalVideoLinks = promoLinks
	}

	return s.db.Create(&newEvent).Error
}

func (s *Service) GetEventList(eventFilter utils.Filter) ([]dto.EventListItemDTO, int64, error) {
	var events []models.Event
	var total int64

	// Count without LIMIT/OFFSET so total reflects the full filtered set.
	countQuery := utils.ApplyEventListBaseFilters(s.db.Model(&models.Event{}), eventFilter)
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Data query applies pagination on top of the same filters.
	query := utils.ApplyEventListFilters(s.db.Model(&models.Event{}).Preload("Thumbnail"), eventFilter)
	if err := query.Find(&events).Error; err != nil {
		return nil, 0, err
	}

	var eventList []dto.EventListItemDTO
	for _, event := range events {
		status := ""
		if event.Status != nil {
			status = *event.Status
		}
		item := dto.EventListItemDTO{
			ID:        event.ID,
			Title:     event.Title,
			Status:    status,
			IsOnline:  event.IsOnline,
			IsPaid:    event.IsPaid,
			StartDate: event.StartDate,
			EndDate:   event.EndDate,
		}
		if event.Thumbnail != nil {
			item.ThumbnailURL = &event.Thumbnail.URL
		}
		eventList = append(eventList, item)
	}

	return eventList, total, nil
}

func (s *Service) UpdateEvent(id string, newData *dto.EventUpdateRequestDTO) error {
	var event models.Event
	if err := s.db.
		Preload("VideoLinks").
		Preload("PromotionalVideoLinks").
		First(&event, "id = ?", id).Error; err != nil {
		return err
	}
	event.Title = newData.Title
	event.Description = newData.Description
	event.Perks = datatypes.JSON(newData.Perks)
	event.StartDate = newData.StartDate
	event.EndDate = newData.EndDate
	event.StartTime = newData.StartTime
	event.EndTime = newData.EndTime
	event.Location = newData.Location
	event.Status = newData.Status
	if newData.IsOnline != nil {
		event.IsOnline = *newData.IsOnline
	} else {
		event.IsOnline = false
	}
	if newData.IsPaid != nil {
		event.IsPaid = *newData.IsPaid
		if *newData.IsPaid {
			if newData.Price != nil {
				event.Price = newData.Price
			} else {
				return utils.NewValidationError("price is required for paid events")
			}
		} else {
			event.Price = nil
		}
	} else {
		event.IsPaid = false
		event.Price = nil
	}
	if newData.RegistrationURL != nil {
		event.RegistrationURL = *newData.RegistrationURL
	}

	if newData.DeletedThumbnailId != nil && *newData.DeletedThumbnailId != "" {
		if err := s.db.Delete(&models.Media{}, "id = ?", *newData.DeletedThumbnailId).Error; err != nil {
			return err
		}
		if err := s.s3.Delete(*newData.DeletedThumbnailId); err != nil {
			log.Printf("S3 cleanup failed for deleted thumbnail %s: %v", *newData.DeletedThumbnailId, err)
		}
		event.ThumbnailID = nil
		event.Thumbnail = nil
	}
	if newData.ThumbnailUpload != "" {
		ref, err := parseMediaRef(newData.ThumbnailUpload)
		if err != nil {
			return fmt.Errorf("invalid thumbnailUpload: %w", err)
		}
		media := s.mediaFromRef(ref)
		if err := s.db.Create(&media).Error; err != nil {
			return err
		}
		event.ThumbnailID = &media.ID
		event.Thumbnail = &media
	}

	if len(newData.Audiences) > 0 {
		audienceRows, err := s.getAudience(newData.Audiences)
		if err != nil {
			return err
		}
		if err := s.db.Model(&event).Association("Audiences").Replace(audienceRows); err != nil {
			return err
		}
	}

	for _, mediaID := range newData.DeletedPromotionalMediaIds {
		if err := s.db.Model(&event).Association("PromotionalMedia").Unscoped().Delete(&models.Media{ID: mediaID}); err != nil {
			log.Printf("failed to unassociate promotional media %s: %v", mediaID, err)
		}
		if err := s.db.Delete(&models.Media{}, "id = ?", mediaID).Error; err != nil {
			return err
		}
		if err := s.s3.Delete(mediaID); err != nil {
			log.Printf("S3 cleanup failed for deleted promotional media %s: %v", mediaID, err)
		}
	}
	for _, mediaID := range newData.DeletedMediaIds {
		if err := s.db.Model(&event).Association("Medias").Unscoped().Delete(&models.Media{ID: mediaID}); err != nil {
			log.Printf("failed to unassociate media %s: %v", mediaID, err)
		}
		if err := s.db.Delete(&models.Media{}, "id = ?", mediaID).Error; err != nil {
			return err
		}
		if err := s.s3.Delete(mediaID); err != nil {
			log.Printf("S3 cleanup failed for deleted media %s: %v", mediaID, err)
		}
	}
	for _, docID := range newData.DeletedDocumentIds {
		if err := s.db.Model(&event).Association("Documents").Unscoped().Delete(&models.Media{ID: docID}); err != nil {
			log.Printf("failed to unassociate document %s: %v", docID, err)
		}
		if err := s.db.Delete(&models.Media{}, "id = ?", docID).Error; err != nil {
			return err
		}
		if err := s.s3.Delete(docID); err != nil {
			log.Printf("S3 cleanup failed for deleted document %s: %v", docID, err)
		}
	}
	for _, docID := range newData.DeletedPromotionalDocumentIds {
		if err := s.db.Model(&event).Association("PromotionalDocuments").Unscoped().Delete(&models.Media{ID: docID}); err != nil {
			log.Printf("failed to unassociate promotional document %s: %v", docID, err)
		}
		if err := s.db.Delete(&models.Media{}, "id = ?", docID).Error; err != nil {
			return err
		}
		if err := s.s3.Delete(docID); err != nil {
			log.Printf("S3 cleanup failed for deleted promotional document %s: %v", docID, err)
		}
	}

	if len(newData.PromotionalMediaUploads) > 0 {
		refs, err := parseMediaRefs(newData.PromotionalMediaUploads)
		if err != nil {
			return fmt.Errorf("invalid promotionalMediaUploads: %w", err)
		}
		if err := s.db.Model(&event).Association("PromotionalMedia").Append(s.mediasFromRefs(refs)); err != nil {
			return err
		}
	}
	if len(newData.PromotionalDocumentUploads) > 0 {
		refs, err := parseMediaRefs(newData.PromotionalDocumentUploads)
		if err != nil {
			return fmt.Errorf("invalid promotionalDocumentUploads: %w", err)
		}
		if err := s.db.Model(&event).Association("PromotionalDocuments").Append(s.mediasFromRefs(refs)); err != nil {
			return err
		}
	}
	if len(newData.DocumentUploads) > 0 {
		refs, err := parseMediaRefs(newData.DocumentUploads)
		if err != nil {
			return fmt.Errorf("invalid documentUploads: %w", err)
		}
		if err := s.db.Model(&event).Association("Documents").Append(s.mediasFromRefs(refs)); err != nil {
			return err
		}
	}
	if len(newData.MediaUploads) > 0 {
		refs, err := parseMediaRefs(newData.MediaUploads)
		if err != nil {
			return fmt.Errorf("invalid mediaUploads: %w", err)
		}
		if err := s.db.Model(&event).Association("Medias").Append(s.mediasFromRefs(refs)); err != nil {
			return err
		}
	}

	// Replace video links: clear old association + delete old Link rows, then create new ones.
	oldVideoLinks := event.VideoLinks
	if err := s.db.Model(&event).Association("VideoLinks").Clear(); err != nil {
		return err
	}
	for _, link := range oldVideoLinks {
		if err := s.db.Delete(&models.Link{}, "id = ?", link.ID).Error; err != nil {
			log.Printf("failed to delete video link %s: %v", link.ID, err)
		}
	}
	if len(newData.VideoLinks) > 0 {
		newLinks, err := s.getLink(newData.VideoLinks)
		if err != nil {
			return err
		}
		if err := s.db.Model(&event).Association("VideoLinks").Append(newLinks); err != nil {
			return err
		}
	}

	// Replace promotional video links the same way.
	oldPromoLinks := event.PromotionalVideoLinks
	if err := s.db.Model(&event).Association("PromotionalVideoLinks").Clear(); err != nil {
		return err
	}
	for _, link := range oldPromoLinks {
		if err := s.db.Delete(&models.Link{}, "id = ?", link.ID).Error; err != nil {
			log.Printf("failed to delete promotional video link %s: %v", link.ID, err)
		}
	}
	if len(newData.PromotionalVideoLinks) > 0 {
		newLinks, err := s.getLink(newData.PromotionalVideoLinks)
		if err != nil {
			return err
		}
		if err := s.db.Model(&event).Association("PromotionalVideoLinks").Append(newLinks); err != nil {
			return err
		}
	}

	return s.db.Save(&event).Error
}

// getAudience fetches audience records matching the given names in a single query.
func (s *Service) getAudience(audiences []string) ([]models.Audience, error) {
	var audienceRows []models.Audience
	if err := s.db.Where("name IN ?", audiences).Find(&audienceRows).Error; err != nil {
		return nil, err
	}
	return audienceRows, nil
}

func (s *Service) DeleteEvent(id string) error {
	var event models.Event
	if err := s.db.
		Preload("Thumbnail").
		Preload("PromotionalMedia").
		Preload("PromotionalDocuments").
		Preload("Medias").
		Preload("Documents").
		Preload("VideoLinks").
		Preload("PromotionalVideoLinks").
		Preload("Audiences").
		First(&event, "id = ?", id).Error; err != nil {
		return err
	}

	allMedia := append(append(append(event.PromotionalMedia, event.PromotionalDocuments...), event.Medias...), event.Documents...)
	allLinks := append(event.VideoLinks, event.PromotionalVideoLinks...)

	// Clear all junction-table associations first.
	if err := s.db.Model(&event).Association("PromotionalMedia").Clear(); err != nil {
		return err
	}
	if err := s.db.Model(&event).Association("PromotionalDocuments").Clear(); err != nil {
		return err
	}
	if err := s.db.Model(&event).Association("Medias").Clear(); err != nil {
		return err
	}
	if err := s.db.Model(&event).Association("Documents").Clear(); err != nil {
		return err
	}
	if err := s.db.Model(&event).Association("VideoLinks").Clear(); err != nil {
		return err
	}
	if err := s.db.Model(&event).Association("PromotionalVideoLinks").Clear(); err != nil {
		return err
	}
	if err := s.db.Model(&event).Association("Audiences").Clear(); err != nil {
		return err
	}

	if event.Thumbnail != nil {
		thumbnailID := event.Thumbnail.ID
		event.ThumbnailID = nil
		if err := s.db.Save(&event).Error; err != nil {
			return err
		}
		if err := s.db.Delete(&models.Media{}, "id = ?", thumbnailID).Error; err != nil {
			return err
		}
		// Best-effort: DB record is gone. Log but don't fail the delete if S3 is unavailable.
		if err := s.s3.Delete(thumbnailID); err != nil {
			log.Printf("S3 delete failed for thumbnail %s: %v", thumbnailID, err)
		}
	}

	for _, media := range allMedia {
		if err := s.db.Delete(&models.Media{}, "id = ?", media.ID).Error; err != nil {
			return err
		}
		// Best-effort S3 cleanup.
		if err := s.s3.Delete(media.ID); err != nil {
			log.Printf("S3 delete failed for media %s: %v", media.ID, err)
		}
	}

	for _, link := range allLinks {
		if err := s.db.Delete(&models.Link{}, "id = ?", link.ID).Error; err != nil {
			return err
		}
	}

	return s.db.Delete(&event).Error
}

func (s *Service) GetHomePageImages() ([]string, error) {
	var upcomingEvents []models.Event
	var pastEvents []models.Event

	if err := s.db.Model(&models.Event{}).
		Where("status = ?", "upcoming").
		Preload("Thumbnail").
		Order("start_date ASC").
		Limit(3).
		Find(&upcomingEvents).Error; err != nil {
		return nil, err
	}

	if err := s.db.Model(&models.Event{}).
		Where("status = ?", "completed").
		Preload("Thumbnail").
		Order("end_date DESC").
		Limit(3).
		Find(&pastEvents).Error; err != nil {
		return nil, err
	}

	var urls []string
	for _, e := range upcomingEvents {
		if e.Thumbnail != nil && e.Thumbnail.URL != "" {
			urls = append(urls, e.Thumbnail.URL)
		}
	}
	for _, e := range pastEvents {
		if e.Thumbnail != nil && e.Thumbnail.URL != "" {
			urls = append(urls, e.Thumbnail.URL)
		}
	}
	return urls, nil
}

func (s *Service) GetUpcomingCarouselImages() ([]string, error) {
	var events []models.Event
	if err := s.db.Model(&models.Event{}).
		Where("status = ?", "upcoming").
		Preload("Thumbnail").
		Order("start_date ASC").
		Limit(4).
		Find(&events).Error; err != nil {
		return nil, err
	}

	var urls []string
	for _, e := range events {
		if e.Thumbnail != nil && e.Thumbnail.URL != "" {
			urls = append(urls, e.Thumbnail.URL)
		}
	}
	return urls, nil
}

func (s *Service) GetCompletedCarouselImages() ([]string, error) {
	var events []models.Event
	if err := s.db.Model(&models.Event{}).
		Where("status = ?", "completed").
		Preload("Medias").
		Preload("PromotionalMedia").
		Order("end_date DESC").
		Limit(4).
		Find(&events).Error; err != nil {
		return nil, err
	}

	var urls []string
	for _, e := range events {
		// Prefer Medias (photos uploaded when the event was completed).
		// Fall back to PromotionalMedia for events that were originally created as
		// upcoming (with promo images) and later flipped to completed.
		candidates := e.Medias
		if len(candidates) == 0 {
			candidates = e.PromotionalMedia
		}
		count := 0
		for _, m := range candidates {
			if count >= 3 {
				break
			}
			if m.URL != "" {
				urls = append(urls, m.URL)
				count++
			}
		}
	}
	return urls, nil
}

func (s *Service) mediaFromRef(ref dto.UploadedMediaRef) models.Media {
	return models.Media{
		ID:       ref.ID,
		Name:     ref.Name,
		URL:      s.s3.PublicURL(ref.ID),
		FileType: ref.FileType,
	}
}

func (s *Service) mediasFromRefs(refs []dto.UploadedMediaRef) []models.Media {
	medias := make([]models.Media, len(refs))
	for i, ref := range refs {
		medias[i] = s.mediaFromRef(ref)
	}
	return medias
}

func parseMediaRef(jsonStr string) (dto.UploadedMediaRef, error) {
	var ref dto.UploadedMediaRef
	if err := json.Unmarshal([]byte(jsonStr), &ref); err != nil {
		return ref, fmt.Errorf("invalid media ref JSON: %w", err)
	}
	return ref, nil
}

func parseMediaRefs(jsonStrs []string) ([]dto.UploadedMediaRef, error) {
	refs := make([]dto.UploadedMediaRef, 0, len(jsonStrs))
	for _, s := range jsonStrs {
		ref, err := parseMediaRef(s)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func (s *Service) getLink(links []string) ([]models.Link, error) {
	var videoLinks []models.Link
	for _, link := range links {
		link := models.Link{
			ID:  uuid.Must(uuid.NewV7()).String(),
			URL: link,
		}
		videoLinks = append(videoLinks, link)
	}
	return videoLinks, nil
}
