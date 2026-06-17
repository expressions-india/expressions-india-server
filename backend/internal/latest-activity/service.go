package latestactivity

import (
	"sort"

	"github.com/dhruvpurohit2k/expressions-india-backend/internal/dto"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/models"
	"gorm.io/gorm"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// GetLatestActivity returns the 5 most-recently created items across all
// content types. Instead of a UNION ALL full-table scan (which Turso counts
// as reading every row in every table), we run one small indexed query per
// table (ORDER BY created_at DESC LIMIT 5), merge the ≤25 results in Go,
// and return the top 5. Row reads: at most 5 per table = 25 total.
func (s *Service) GetLatestActivity() ([]dto.LatestActivity, error) {
	const n = 5

	var activities []dto.LatestActivity

	// Events (soft-delete aware)
	var events []models.Event
	if err := s.db.Select("id, title, start_date, end_date, created_at").
		Order("created_at DESC").Limit(n).Find(&events).Error; err != nil {
		return nil, err
	}
	for _, e := range events {
		activities = append(activities, dto.LatestActivity{
			Type:      "event",
			ID:        e.ID,
			Title:     e.Title,
			StartDate: e.StartDate,
			EndDate:   e.EndDate,
			CreatedAt: e.CreatedAt,
		})
	}

	// Articles
	var articles []models.Article
	if err := s.db.Select("id, title, created_at").
		Order("created_at DESC").Limit(n).Find(&articles).Error; err != nil {
		return nil, err
	}
	for _, a := range articles {
		activities = append(activities, dto.LatestActivity{
			Type:      "article",
			ID:        a.ID,
			Title:     a.Title,
			CreatedAt: a.CreatedAt,
		})
	}

	// Journals
	var journals []models.Journal
	if err := s.db.Select("id, title, created_at").
		Order("created_at DESC").Limit(n).Find(&journals).Error; err != nil {
		return nil, err
	}
	for _, j := range journals {
		activities = append(activities, dto.LatestActivity{
			Type:      "journal",
			ID:        j.ID,
			Title:     j.Title,
			CreatedAt: j.CreatedAt,
		})
	}

	// Podcasts
	var podcasts []models.Podcast
	if err := s.db.Select("id, title, created_at").
		Order("created_at DESC").Limit(n).Find(&podcasts).Error; err != nil {
		return nil, err
	}
	for _, p := range podcasts {
		activities = append(activities, dto.LatestActivity{
			Type:      "podcast",
			ID:        p.ID,
			Title:     p.Title,
			CreatedAt: p.CreatedAt,
		})
	}

	// Courses
	var courses []models.Course
	if err := s.db.Select("id, title, created_at").
		Order("created_at DESC").Limit(n).Find(&courses).Error; err != nil {
		return nil, err
	}
	for _, c := range courses {
		activities = append(activities, dto.LatestActivity{
			Type:      "course",
			ID:        c.ID,
			Title:     c.Title,
			CreatedAt: c.CreatedAt,
		})
	}

	// Sort all ≤25 candidates in Go and return the top 5.
	sort.Slice(activities, func(i, j int) bool {
		return activities[i].CreatedAt.After(activities[j].CreatedAt)
	})
	if len(activities) > n {
		activities = activities[:n]
	}

	// Ensure nil slice is returned as empty slice so JSON encodes as [].
	if activities == nil {
		return []dto.LatestActivity{}, nil
	}
	return activities, nil
}
