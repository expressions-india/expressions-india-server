package main

import (
	"fmt"
	"log"
	"os"

	"strings"
	"time"

	"github.com/dhruvpurohit2k/expressions-india-backend/internal/almanac"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/article"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/audience"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/auth"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/brochure"
	certificateapplication "github.com/dhruvpurohit2k/expressions-india-backend/internal/certificateapplication"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/course"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/enquiry"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/event"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/journal"
	latestactivity "github.com/dhruvpurohit2k/expressions-india-backend/internal/latest-activity"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/models"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/pkg/utils"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/podcast"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/promotion"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/purchase"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/storage"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/team"
	"github.com/dhruvpurohit2k/expressions-india-backend/internal/upload"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Server struct {
	r                                *gin.Engine
	db                               *gorm.DB
	s3                               *storage.S3
	eventController                  *event.Controller
	promotionController              *promotion.Controller
	journalController                *journal.Controller
	podcastController                *podcast.Controller
	enquiryController                *enquiry.Controller
	articleController                *article.Controller
	audienceController               *audience.Controller
	latestActivityController         *latestactivity.Controller
	courseController                 *course.Controller
	authController                   *auth.Controller
	teamController                   *team.Controller
	uploadController                 *upload.Controller
	almanacController                *almanac.Controller
	brochureController               *brochure.Controller
	certificateApplicationController *certificateapplication.Controller
	purchaseController               *purchase.Controller
}

func initServer() *Server {
	if env := os.Getenv("APP_ENV"); env == "production" || env == "staging" {
		gin.SetMode(gin.ReleaseMode)
	}
	fmt.Println("INITDB STARTING")
	db := storage.InitDB()
	s3 := storage.InitS3()

	if os.Getenv("DB_FRESH_START") == "true" && os.Getenv("APP_ENV") == "development" {
		db.Migrator().DropTable(
			&models.User{},
			&models.Event{},
			&models.Media{},
			&models.Promotion{},
			&models.Link{},
			&models.Journal{},
			&models.JournalChapter{},
			&models.Author{},
			&models.Podcast{},
			&models.Enquiry{},
			&models.Article{},
			&models.Course{},
			&models.CourseChapter{},
			&models.Team{},
			&models.Member{},
			&models.Audience{},
		)
	}
	err := db.AutoMigrate(
		&models.User{},
		&models.Event{},
		&models.Media{},
		&models.Audience{},
		&models.Promotion{},
		&models.Link{},
		&models.Journal{},
		&models.JournalChapter{},
		&models.Author{},
		&models.Podcast{},
		&models.Enquiry{},
		&models.Article{},
		&models.Course{},
		&models.CourseChapter{},
		&models.Team{},
		&models.Member{},
		&models.Almanac{},
		&models.Brochure{},
		&models.CertificateApplication{},
	)
	if err != nil {
		log.Fatal(err.Error())
	}

	models.SeedAudience(db)

	eventService := event.NewService(db, s3)
	eventController := event.NewController(eventService)

	promotionService := promotion.NewService(db)
	promotionController := promotion.NewController(promotionService)

	journalsService := journal.NewService(db, s3)
	journalsController := journal.NewController(journalsService)

	podcastService := podcast.NewService(db)
	podcastController := podcast.NewController(podcastService)

	enquiryService := enquiry.NewService(db)
	enquiryController := enquiry.NewController(enquiryService)

	articleService := article.NewService(db, s3)
	articleController := article.NewController(articleService)

	audienceService := audience.NewService(db)
	audienceController := audience.NewController(audienceService)

	latestActivityService := latestactivity.NewService(db)
	latestActivityController := latestactivity.NewController(latestActivityService)

	courseService := course.NewService(db, s3)
	courseController := course.NewController(courseService)

	authService := auth.NewService(db)
	authController := auth.NewController(authService)

	teamService := team.NewService(db)
	teamController := team.NewController(teamService)

	uploadController := upload.NewController(s3)

	almanacService := almanac.NewService(db, s3)
	almanacController := almanac.NewController(almanacService)

	brochureService := brochure.NewService(db, s3)
	brochureController := brochure.NewController(brochureService)

	certAppService := certificateapplication.NewService(db)
	certAppController := certificateapplication.NewController(certAppService)

	revenueCatClient := purchase.NewRevenueCatClient()
	purchaseController := purchase.NewController(revenueCatClient, courseService)

	var r *gin.Engine
	if env := os.Getenv("APP_ENV"); env == "production" || env == "staging" {
		r = gin.New()
		r.Use(gin.Recovery())
	} else {
		r = gin.Default()
	}
	corsConfig := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		MaxAge:           12 * time.Hour,
		AllowCredentials: true,
	}
	if allowedOrigins := os.Getenv("ALLOWED_ORIGINS"); allowedOrigins != "" {
		parts := strings.Split(allowedOrigins, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		corsConfig.AllowOrigins = parts
	} else {
		// Fallback: allow common local dev servers.
		corsConfig.AllowOrigins = []string{"http://localhost:5173", "http://localhost:3000", "http://localhost:8081"}
	}
	r.Use(cors.New(corsConfig))
	r.Use(MaxBodyBytes(2 << 20)) // 2 MB cap on JSON/form bodies (file uploads go to S3 directly)
	return &Server{
		r:                                r,
		db:                               db,
		s3:                               s3,
		eventController:                  eventController,
		promotionController:              promotionController,
		journalController:                journalsController,
		podcastController:                podcastController,
		enquiryController:                enquiryController,
		articleController:                articleController,
		audienceController:               audienceController,
		latestActivityController:         latestActivityController,
		courseController:                 courseController,
		authController:                   authController,
		teamController:                   teamController,
		uploadController:                 uploadController,
		almanacController:                almanacController,
		brochureController:               brochureController,
		certificateApplicationController: certAppController,
		purchaseController:               purchaseController,
	}
}

func (s *Server) SetupRoutes() {
	s.r.GET("/hello", func(ctx *gin.Context) {
		utils.OK(ctx, gin.H{
			"message": "Success",
		})
	})

	// Per-IP rate limit for unauthenticated/abuse-prone endpoints: 1 req/sec, burst 5.
	publicLimit := RateLimit(1, 5)

	groupAuth := s.r.Group("/auth")
	{
		// Password login — admins only (service rejects non-password accounts).
		groupAuth.POST("/login", publicLimit, s.authController.Login)
		// OIDC sign-in — end users on Expo (Android: Google; iOS: Google + Apple).
		groupAuth.POST("/google", publicLimit, s.authController.Google)
		groupAuth.POST("/apple", publicLimit, s.authController.Apple)
		groupAuth.POST("/refresh", publicLimit, s.authController.Refresh)
		groupAuth.POST("/logout", s.authController.Logout)
		// Register is admin-only: only an existing admin can create new password admins.
		groupAuth.POST("/register", auth.RequireAdmin(), s.authController.Register)
	}

	groupAdmin := s.r.Group("/api/admin", auth.RequireAdmin())
	{
		groupAdmin.POST("/upload/presign", s.uploadController.Presign)

		groupAdmin.GET("/allEvents", s.eventController.GetAll)
		groupAdmin.GET("/event", s.eventController.GetEventList)
		groupAdmin.GET("/event/:id", s.eventController.GetEventById)
		groupAdmin.POST("/event", s.eventController.Create)
		groupAdmin.PUT("/event/:id", s.eventController.Update)
		groupAdmin.DELETE("/event/:id", s.eventController.Delete)

		groupAdmin.GET("/journal", s.journalController.GetList)
		groupAdmin.GET("/journal/:id", s.journalController.GetById)
		groupAdmin.POST("/journal", s.journalController.Create)
		groupAdmin.PUT("/journal/:id", s.journalController.Update)
		groupAdmin.DELETE("/journal/:id", s.journalController.Delete)
		groupAdmin.GET("/promotion", s.promotionController.Get)
		groupAdmin.GET("/promotion/:id", s.promotionController.GetById)

		groupAdmin.GET("/podcast", s.podcastController.GetPodcastList)
		groupAdmin.GET("/podcast/:id", s.podcastController.GetById)
		groupAdmin.POST("/podcast", s.podcastController.Create)
		groupAdmin.PUT("/podcast/:id", s.podcastController.Update)
		groupAdmin.DELETE("/podcast/:id", s.podcastController.Delete)

		groupAdmin.GET("/enquiry", s.enquiryController.GetList)
		groupAdmin.GET("/enquiry/:id", s.enquiryController.GetById)
		groupAdmin.DELETE("/enquiry/:id", s.enquiryController.Delete)

		groupAdmin.GET("/audience", s.audienceController.GetAudience)
		groupAdmin.PUT("/audience/:id", s.audienceController.UpdateDescription)
		// groupAdmin.GET("/audience/:id", s.audienceController.GetById)
		// groupAdmin.POST("/audience", s.audienceController.Create)
		// groupAdmin.DELETE("/audience/:id", s.audienceController.Delete)

		groupAdmin.GET("/article", s.articleController.GetArticleList)
		groupAdmin.GET("/article/:id", s.articleController.GetArticleById)
		groupAdmin.POST("/article", s.articleController.Create)
		groupAdmin.PUT("/article/:id", s.articleController.Update)
		groupAdmin.DELETE("/article/:id", s.articleController.Delete)

		groupAdmin.GET("/course", s.courseController.GetCoursesListAdmin)
		groupAdmin.GET("/course/:id", s.courseController.GetCourseByIdAdmin)
		groupAdmin.POST("/course", s.courseController.Create)
		groupAdmin.PUT("/course/:id", s.courseController.Update)
		groupAdmin.DELETE("/course/:id", s.courseController.Delete)
		groupAdmin.GET("/course/:id/enrolled", s.courseController.GetEnrolledUsers)
		groupAdmin.DELETE("/course/:id/enrolled/:userId", s.courseController.RevokeAccess)
		groupAdmin.GET("/course/:id/not-enrolled", s.courseController.GetNotEnrolledUsers)
		groupAdmin.POST("/course/:id/enroll", s.courseController.EnrollUser)

		groupAdmin.GET("/team", s.teamController.GetList)
		groupAdmin.GET("/team/:id", s.teamController.GetById)
		groupAdmin.POST("/team", s.teamController.Create)
		groupAdmin.PUT("/team/:id", s.teamController.Update)
		groupAdmin.DELETE("/team/:id", s.teamController.Delete)

		groupAdmin.GET("/almanac", s.almanacController.GetList)
		groupAdmin.GET("/almanac/:id", s.almanacController.GetById)
		groupAdmin.POST("/almanac", s.almanacController.Create)
		groupAdmin.PUT("/almanac/:id", s.almanacController.Update)
		groupAdmin.DELETE("/almanac/:id", s.almanacController.Delete)

		groupAdmin.GET("/brochure", s.brochureController.GetList)
		groupAdmin.GET("/brochure/:id", s.brochureController.GetById)
		groupAdmin.POST("/brochure", s.brochureController.Create)
		groupAdmin.PUT("/brochure/:id", s.brochureController.Update)
		groupAdmin.DELETE("/brochure/:id", s.brochureController.Delete)

		groupAdmin.GET("/certificate-application", s.certificateApplicationController.GetAll)
		groupAdmin.GET("/certificate-application/:id", s.certificateApplicationController.GetByID)
		groupAdmin.POST("/certificate-application", s.certificateApplicationController.Create)
		groupAdmin.PUT("/certificate-application/:id", s.certificateApplicationController.Update)
		groupAdmin.DELETE("/certificate-application/:id", s.certificateApplicationController.Delete)
	}
	groupApi := s.r.Group("/api")
	{
		groupApi.GET("/home/images", s.eventController.GetHomePageImages)
		groupApi.GET("/home/upcoming-images", s.eventController.GetUpcomingCarouselImages)
		groupApi.GET("/home/completed-images", s.eventController.GetCompletedCarouselImages)
		groupApi.GET("/event/upcoming", s.eventController.GetUpcomingEvents)
		groupApi.GET("/event/past", s.eventController.GetPastEvents)
		groupApi.GET("/event/:id", s.eventController.GetEventById)
		groupApi.GET("/podcast", s.podcastController.GetPodcastList)
		groupApi.GET("/podcast/:id", s.podcastController.GetById)
		groupApi.GET("/journal", s.journalController.GetList)
		groupApi.GET("/journal/:id", s.journalController.GetById)
		groupApi.POST("/enquiry", publicLimit, s.enquiryController.CreateEnquiry)
		groupApi.GET("/article", s.articleController.GetArticleListPaginated)
		groupApi.GET("/article/audience/:audience", s.articleController.GetArticlesByAudience)
		groupApi.GET("/article/:id", s.articleController.GetArticleById)
		groupApi.GET("/podcast/audience/:audience", s.podcastController.GetPodcastsByAudience)
		groupApi.GET("/event/audience/:audience", s.eventController.GetUpcomingEventsByAudience)
		groupApi.GET("/audience/:name", s.audienceController.GetAudienceByName)
		groupApi.GET("/latest-activity", s.latestActivityController.GetLatestActivity)
		groupApi.GET("/course", s.courseController.GetCoursesList)
		groupApi.GET("/course/my", auth.RequireAuth(), s.courseController.GetMyCourses)
		groupApi.GET("/course/audience/:audience", s.courseController.GetCoursesByAudience)
		groupApi.GET("/course/:id", s.courseController.GetCourseById)
		groupApi.GET("/course/:id/chapter/:chapterId", auth.TryExtractClaims(), s.courseController.GetChapterById)
		groupApi.GET("/team", s.teamController.GetList)
		groupApi.GET("/team/:id", s.teamController.GetById)

		groupApi.GET("/almanac", s.almanacController.GetList)
		groupApi.GET("/almanac/:id", s.almanacController.GetById)

		groupApi.GET("/brochure", s.brochureController.GetList)
		groupApi.GET("/brochure/:id", s.brochureController.GetById)

		groupApi.GET("/certificate-application", s.certificateApplicationController.GetPublic)

		groupApi.POST("/course/:id/purchase", auth.RequireAuth(), s.purchaseController.PurchaseCourse)
	}

	// Webhooks — public endpoints with their own authentication (HMAC signatures).
	s.r.POST("/api/webhooks/revenuecat", s.purchaseController.RevenueCatWebhook)

}
