package api

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/matthewhartstonge/argon2"
	"log/slog"
	"net/http"
	"supadash/conf"
	"supadash/database"
	"supadash/provisioner"
	"time"
)

type Api struct {
	isHealthy       bool
	logger          *slog.Logger
	config          *conf.Config
	queries         Querier
	pgPool          *pgxpool.Pool
	argon           argon2.Config
	provisioner     provisioner.Provisioner
	resourceManager *provisioner.ResourceManager
	burstPool       *provisioner.BurstPoolManager
}

func CreateApi(logger *slog.Logger, config *conf.Config) (*Api, error) {
	conn, err := pgxpool.New(context.Background(), config.DatabaseUrl)
	if err != nil {
		logger.Error(fmt.Sprintf("Unable to connect to database: %v", err))
		return nil, err
	}

	if err := conf.EnsureMigrationsTableExists(conn); err != nil {
		logger.Error(fmt.Sprintf("Failed to ensure migrations table: %v", err))
		return nil, err
	}

	queries := database.New(conn)

	if success, err := conf.EnsureMigrations(conn, queries); err != nil || !success {
		logger.Error(fmt.Sprintf("Failed to run migrations: %v", err))
		return nil, err
	}

	// Initialize provisioner if enabled
	var prov provisioner.Provisioner
	var resMgr *provisioner.ResourceManager
	var burstMgr *provisioner.BurstPoolManager
	if config.Provisioning.Enabled {
		// Use NewDockerProvisioner with projects directory and templates directory
		dockerProv, err := provisioner.NewDockerProvisioner(
			config.Provisioning.ProjectsDir,
			"./templates",
			logger,
		)
		if err != nil {
			logger.Warn(fmt.Sprintf("Failed to initialize provisioner: %v", err))
			logger.Info("Continuing without provisioner - projects can be created but not provisioned")
		} else {
			prov = dockerProv
			logger.Info("Docker provisioner initialized and enabled")

			// Initialize resource manager and burst pool
			resMgr = provisioner.NewResourceManager(logger, dockerProv)
			burstMgr = provisioner.NewBurstPoolManager(logger, 8*1024*1024*1024) // 8GB default

			// Start analysis collector in the background
			collector := provisioner.NewAnalysisCollector(logger, queries, dockerProv, burstMgr)
			go collector.Run(context.Background())
			logger.Info("Analysis collector started")
		}
	} else {
		logger.Info("Provisioner is disabled")
	}

	return &Api{
		logger:          logger,
		config:          config,
		queries:         queries,
		pgPool:          conn,
		argon:           argon2.DefaultConfig(),
		provisioner:     prov,
		resourceManager: resMgr,
		burstPool:       burstMgr,
	}, nil
}

func (a *Api) GetAccountIdFromRequest(c *gin.Context) (string, error) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return "", errors.New("missing Authorization header")
	}

	tokenString := authHeader[len("Bearer "):]
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(a.config.JwtSecret), nil
	})
	if err != nil {
		return "", err
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok {
		return "", errors.New("invalid token claims")
	}

	return claims.Subject, nil
}

func (a *Api) GetAccountFromRequest(c *gin.Context) (*database.Account, error) {
	id, err := a.GetAccountIdFromRequest(c)
	if err != nil {
		return nil, err
	}

	if id == "" {
		return nil, errors.New("missing account ID")
	}

	account, err := a.queries.GetAccountByGoTrueID(c.Request.Context(), id)
	if err != nil {
		return nil, err
	}

	return &account, nil
}

func (a *Api) ListenAddress() string {
	return ":8080"
}

func (a *Api) index(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "OK"})
}

func (a *Api) status(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"is_healthy": a.isHealthy})
}

func (a *Api) telemetry(c *gin.Context) {
	c.AbortWithStatus(http.StatusNoContent)
}

const INDEX = ""

func (a *Api) Router() *gin.Engine {
	r := gin.Default()

	// Construct allowed CORS origins
	allowedOrigins := a.config.AllowedOrigins
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"*"}
	}

	r.Use(cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Authorization", "Content-Type", "Accept"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Apply Rate Limiting globally (burst is 2x limit)
	r.Use(RateLimitMiddleware(float64(a.config.RateLimitRequests), a.config.RateLimitRequests*2))

	r.GET("/", a.index)
	r.GET("/status", a.status)

	profile := r.Group("/profile")
	{
		profile.GET(INDEX, a.getProfile)
		profile.GET("/permissions", a.getProfilePermissions)
		profile.POST("/password-check", a.postPasswordCheck)
	}

	organization := r.Group("/organizations")
	{
		organization.GET(INDEX, a.getOrganizations)

		specificOrganization := organization.Group("/:slug")
		specificOrganization.Use(a.RequireOrgRole("owner", "admin", "member", "viewer"))
		{
			members := specificOrganization.Group("/members")
			{
				members.GET("/reached-free-project-limit", a.getOrganizationMembersReachedFreeProjectLimit)
			}
			
			// Team Management
			team := specificOrganization.Group("/team")
			{
				team.GET(INDEX, a.getTeamMembers)
				team.POST("/invite", a.RequireOrgRole("owner", "admin"), a.inviteTeamMember)
				team.PUT("/:id", a.RequireOrgRole("owner", "admin"), a.updateTeamMemberRole)
				team.DELETE("/:id", a.RequireOrgRole("owner", "admin"), a.removeTeamMember)
			}
		}
	}

	projects := r.Group("/projects")
	{
		specificProject := projects.Group("/:ref")
		{
			specificProject.GET("/status", a.getProjectStatus)
			specificProject.GET("/jwt-secret-update-status", a.getProjectJwtSecretUpdateStatus)
			specificProject.GET("/api", a.getProjectApi)
			specificProject.GET("/upgrade/status", a.getProjectUpgradeStatus)
			specificProject.GET("/health", a.getProjectHealth)
			specificProject.GET("/supervisor", a.getProjectSupervisor)
			
			specificProject.POST("/pause", a.RequireProjectRole("owner", "admin"), a.postProjectPause)
			specificProject.POST("/resume", a.RequireProjectRole("owner", "admin"), a.postProjectResume)
			specificProject.DELETE(INDEX, a.RequireProjectRole("owner"), a.deleteProject)

			// Env var management
			specificProject.GET("/env", a.RequireProjectRole("owner", "admin", "member", "viewer"), a.getProjectEnvVars)
			specificProject.PUT("/env", a.RequireProjectRole("owner", "admin"), a.putProjectEnvVars)

			// Resource management
			specificProject.GET("/resources", a.RequireProjectRole("owner", "admin", "member", "viewer"), a.getProjectResources)
			specificProject.PUT("/resources", a.RequireProjectRole("owner", "admin"), a.putProjectResources)

			// Secrets management
			specificProject.POST("/secrets/rotate", a.RequireProjectRole("owner", "admin"), a.rotateProjectSecret)

			// Audit Logs
			specificProject.GET("/audit", a.RequireProjectRole("owner", "admin", "member", "viewer"), a.getProjectAuditLogs)

			// Analysis
			specificProject.GET("/analysis", a.getProjectAnalysis)
			specificProject.GET("/analysis/history", a.getProjectAnalysisHistory)
			specificProject.GET("/analysis/recommendations", a.getProjectRecommendations)
			specificProject.POST("/analysis/recommendations/:id/dismiss", a.dismissRecommendation)

			// Analytics routes
			analytics := specificProject.Group("/analytics/endpoints")
			{
				analytics.GET("/usage.api-counts", a.getProjectAnalyticsEndpointUsage)
				analytics.GET("/usage.api-requests-count", a.getProjectAnalyticsEndpointUsage)
			}
		}
	}

	// Singular /project routes (some Studio UI calls use singular)
	project := r.Group("/project")
	{
		specificProject := project.Group("/:ref")
		{
			specificProject.GET("/status", a.getProjectStatus)
			specificProject.GET("/jwt-secret-update-status", a.getProjectJwtSecretUpdateStatus)
			specificProject.GET("/api", a.getProjectApi)
			specificProject.GET("/health", a.getProjectHealth)
			specificProject.GET("/supervisor", a.getProjectSupervisor)
		}
	}

	// Props routes (used by Studio UI)
	props := r.Group("/props")
	{
		propsProject := props.Group("/project")
		{
			specificProject := propsProject.Group("/:ref")
			{
				specificProject.GET("/jwt-secret-update-status", a.getPropsProjectJwtSecretUpdateStatus)
			}
		}
	}

	gotrue := r.Group("/auth")
	{
		gotrue.POST("/token", a.postGotrueToken)
	}

	platform := r.Group("/platform")
	{
		platform.POST("/signup", a.postPlatformSignup)
		platform.GET("/notifications", a.getPlatformNotifications)
		platform.GET("/notifications/summary", a.getPlatformNotificationsSummary)
		platform.GET("/stripe/invoices/overdue", a.getPlatformOverdueInvoices)
		platform.GET("/projects-resource-warnings", a.getPlatformProjectsResourceWarnings)

		// pg-meta routes for database metadata queries via reverse proxy
		platformPgMeta := platform.Group("/pg-meta")
		{
			// Wildcard route to proxy any request to /platform/pg-meta/:ref/*path
			// to the project's Kong /pg/*path
			platformPgMeta.Any("/:ref/*path", a.anyPlatformPgMetaProxy)
		}

		platformProjects := platform.Group("/projects")
		{
			platformProjects.GET(INDEX, a.getPlatformProjects)
			platformProjects.POST(INDEX, a.postPlatformProjects)
			specificProject := platformProjects.Group("/:ref")
			{
				specificProject.GET(INDEX, a.getPlatformProject)
				specificProject.GET("/settings", a.getPlatformProjectSettings)
				specificProject.GET("/billing/addons", a.getPlatformProjectBillingAddons)
				specificProject.GET("/status", a.getProjectStatus)
				specificProject.GET("/service-versions", a.getPlatformProjectServiceVersions)
				specificProject.GET("/members", a.getPlatformProjectMembers)

				// Analytics routes
				analytics := specificProject.Group("/analytics/endpoints")
				{
					analytics.GET("/usage.api-counts", a.getPlatformProjectAnalyticsEndpointUsage)
					analytics.GET("/usage.api-requests-count", a.getPlatformProjectAnalyticsEndpointUsage)
				}
			}
		}

		// Singular /project routes (some Studio UI calls use singular)
		platformProject := platform.Group("/project")
		{
			specificProject := platformProject.Group("/:ref")
			{
				specificProject.GET(INDEX, a.getPlatformProject)
				specificProject.GET("/settings", a.getPlatformProjectSettings)
				specificProject.GET("/billing/addons", a.getPlatformProjectBillingAddons)
			}
		}

		platformOrganizations := platform.Group("/organizations")
		{
			platformOrganizations.GET(INDEX, a.getOrganizations)
			platformOrganizations.POST(INDEX, a.postPlatformOrganizations)
			specificOrganization := platformOrganizations.Group("/:slug")
			{
				specificOrganization.GET("/projects", a.getPlatformOrganizationProjects)
				specificOrganization.GET("/billing/subscription", a.getPlatformOrganizationSubscription)
				specificOrganization.GET("/usage", a.getPlatformOrganizationUsage)
			}
		}

		platform.GET("/integrations/:integration/connections", a.getIntegrationConnections)
		platform.GET("/integrations/:integration/authorization", a.getPlatformIntegrationAuthorization)
		platform.GET("/integrations/:integration/repositories", a.getPlatformIntegrationRepositories)
	}

	// Integrations routes (organization level)
	integrations := r.Group("/integrations")
	{
		integrations.GET("/:id", a.getIntegrations)
	}

	configcat := r.Group("/configcat")
	{
		configcat.GET("/configuration-files/:key/config_v5.json", a.getConfigCatConfiguration)
	}

	v1 := r.Group("/v1")
	{
		// Monitoring (no auth)
		v1.GET("/health", a.status)
		v1.GET("/metrics", a.getMetrics)

		auth := v1.Group("/auth")
		{
			auth.POST("/token", a.postGotrueToken)
			auth.POST("/logout", a.postAuthLogout)
	
			// MFA endpoints
			auth.POST("/mfa/setup", a.postMfaSetup)
			auth.POST("/mfa/verify", a.postMfaVerify)
			auth.DELETE("/mfa", a.deleteMfa)
		}

		v1Projects := v1.Group("/projects")
		{
			specificProject := v1Projects.Group("/:ref")
			{
				specificProject.GET("/custom-hostname", a.getProjectCustomHostname)
				specificProject.GET("/upgrade/eligibility", a.getProjectUpgradeEligibility)
				
				// Edge Function Secrets management
				specificProject.GET("/secrets", a.getV1ProjectSecrets)
				specificProject.POST("/secrets", a.postV1ProjectSecrets)
				specificProject.DELETE("/secrets", a.deleteV1ProjectSecrets)
			}
		}
	}

	// Server-level resource management
	server := r.Group("/server")
	{
		server.GET("/resources", a.getServerResources)
		server.GET("/resources/capacity", a.getServerCapacity)
	}

	return r
}
