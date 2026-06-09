package v1

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	v1 "github.com/kubev2v/assisted-migration-agent/api/v1"
	"github.com/kubev2v/assisted-migration-agent/internal/models"
	srvErrors "github.com/kubev2v/assisted-migration-agent/pkg/errors"
	"github.com/kubev2v/assisted-migration-agent/pkg/filter"
)

var clusterIDPattern = regexp.MustCompile(`^(domain-c\d+|cluster-[0-9a-f]{16})$`)

// ListRightsizingReports returns all stored rightsizing reports.
// (GET /rightsizing)
func (h *Handler) ListRightsizingReports(c *gin.Context) {
	reports, err := h.rightsizingSrv.ListReports(c.Request.Context())
	if err != nil {
		zap.S().Named("rightsizing_handler").Errorw("failed to list reports", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	apiReports := make([]v1.RightsizingReportSummary, 0, len(reports))
	for _, r := range reports {
		apiReports = append(apiReports, v1.NewRightsizingReportSummaryFromModel(r))
	}

	c.JSON(http.StatusOK, v1.RightsizingReportListResponse{
		Reports: apiReports,
		Total:   len(apiReports),
	})
}

// GetRightsizingReport returns a single rightsizing report by ID.
// (GET /rightsizing/{report_id})
func (h *Handler) GetRightsizingReport(c *gin.Context, reportId string) {
	report, err := h.rightsizingSrv.GetReport(c.Request.Context(), reportId)
	if err != nil {
		if srvErrors.IsResourceNotFoundError(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		zap.S().Named("rightsizing_handler").Errorw("failed to get report", "id", reportId, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, v1.NewRightsizingReportFromModel(*report))
}

// TriggerRightsizingCollection triggers a rightsizing metrics collection run.
// (POST /rightsizing)
func (h *Handler) TriggerRightsizingCollection(c *gin.Context) {
	var req v1.RightsizingCollectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": validationErrorMessage(err)})
		return
	}

	creds, err := v1.CredsFromAPI(req.Credentials)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := creds.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	params := models.RightsizingParams{
		Credentials: creds,
		LookbackH:   defaultInt(req.LookbackHours, 720),
		IntervalID:  defaultInt(req.IntervalId, 7200),
		BatchSize:   defaultInt(req.BatchSize, 64),
	}
	if req.NameFilter != nil {
		params.NameFilter = *req.NameFilter
	}
	if req.ClusterId != nil {
		params.ClusterID = *req.ClusterId
	}
	if req.DiscoverVms != nil {
		params.DiscoverVMs = *req.DiscoverVms
	}

	report, err := h.rightsizingSrv.TriggerCollection(c.Request.Context(), params)
	if err != nil {
		zap.S().Named("rightsizing_handler").Errorw("failed to trigger collection", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, v1.NewRightsizingReportSummaryFromModel(*report))
}

// GetVMUtilization returns utilization details for a specific VM.
// (GET /vms/{id}/utilization)
func (h *Handler) GetVMUtilization(c *gin.Context, id string) {
	details, err := h.rightsizingSrv.GetVMUtilization(c.Request.Context(), id)
	if err != nil {
		if srvErrors.IsResourceNotFoundError(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		zap.S().Named("rightsizing_handler").Errorw("failed to get VM utilization", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, v1.NewVmUtilizationDetailsFromModel(*details))
}

// GetLatestRightsizingClusters returns cluster utilization from the latest completed report.
// (GET /rightsizing/clusters)
func (h *Handler) GetLatestRightsizingClusters(c *gin.Context, params v1.GetLatestRightsizingClustersParams) {
	filterExpr := ""
	if params.ByExpression != nil {
		if _, err := filter.ParseWithClusterMap([]byte(*params.ByExpression)); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("expression filter is invalid: %v", err)})
			return
		}
		filterExpr = *params.ByExpression
	}
	reportID, clusters, err := h.rightsizingSrv.ListLatestClusterUtilization(c.Request.Context(), filterExpr)
	if err != nil {
		zap.S().Named("rightsizing_handler").Errorw("failed to list latest cluster utilization", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if reportID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "no completed rightsizing report found"})
		return
	}

	apiClusters := make([]v1.RightsizingClusterUtilization, 0, len(clusters))
	for _, cl := range clusters {
		apiClusters = append(apiClusters, v1.NewRightsizingClusterUtilizationFromModel(cl))
	}
	c.JSON(http.StatusOK, v1.RightsizingClusterListResponse{
		ReportId: reportID,
		Clusters: apiClusters,
	})
}

// GetRightsizingReportClusters returns cluster utilization for a specific report.
// Deprecated: use ListRightsizingReportClusters (GET /rightsizing/{report_id}/clusters).
// (GET /cluster_rightsizing/{report_id})
func (h *Handler) GetRightsizingReportClusters(c *gin.Context, reportId string, params v1.GetRightsizingReportClustersParams) {
	// Validate report exists first.
	if _, err := h.rightsizingSrv.GetReport(c.Request.Context(), reportId); err != nil {
		if srvErrors.IsResourceNotFoundError(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		zap.S().Named("rightsizing_handler").Errorw("failed to get report", "report_id", reportId, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filterExpr := ""
	if params.ByExpression != nil {
		if _, err := filter.ParseWithClusterMap([]byte(*params.ByExpression)); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("expression filter is invalid: %v", err)})
			return
		}
		filterExpr = *params.ByExpression
	}
	clusters, err := h.rightsizingSrv.ListClusterUtilization(c.Request.Context(), reportId, filterExpr)
	if err != nil {
		zap.S().Named("rightsizing_handler").Errorw("failed to list cluster utilization", "report_id", reportId, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	apiClusters := make([]v1.RightsizingClusterUtilization, 0, len(clusters))
	for _, cl := range clusters {
		apiClusters = append(apiClusters, v1.NewRightsizingClusterUtilizationFromModel(cl))
	}
	c.JSON(http.StatusOK, v1.RightsizingClusterListResponse{
		ReportId: reportId,
		Clusters: apiClusters,
	})
}

func defaultInt(p *int, fallback int) int {
	if p != nil {
		return *p
	}
	return fallback
}

// ListRightsizingReportClusters returns cluster utilization for a specific report.
// (GET /rightsizing/{report_id}/clusters)
func (h *Handler) ListRightsizingReportClusters(c *gin.Context, reportId string, params v1.ListRightsizingReportClustersParams) {
	if _, err := h.rightsizingSrv.GetReport(c.Request.Context(), reportId); err != nil {
		if srvErrors.IsResourceNotFoundError(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		zap.S().Named("rightsizing_handler").Errorw("failed to get report", "report_id", reportId, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filterExpr := ""
	if params.ByExpression != nil {
		if _, err := filter.ParseWithClusterMap([]byte(*params.ByExpression)); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("expression filter is invalid: %v", err)})
			return
		}
		filterExpr = *params.ByExpression
	}
	clusters, err := h.rightsizingSrv.ListClusterUtilization(c.Request.Context(), reportId, filterExpr)
	if err != nil {
		zap.S().Named("rightsizing_handler").Errorw("failed to list cluster utilization", "report_id", reportId, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	apiClusters := make([]v1.RightsizingClusterUtilization, 0, len(clusters))
	for _, cl := range clusters {
		apiClusters = append(apiClusters, v1.NewRightsizingClusterUtilizationFromModel(cl))
	}
	c.JSON(http.StatusOK, v1.RightsizingClusterListResponse{
		ReportId: reportId,
		Clusters: apiClusters,
	})
}

// GetClusterUtilization returns utilization for a specific cluster from the latest completed report.
// (GET /clusters/{cluster_id}/utilization)
func (h *Handler) GetClusterUtilization(c *gin.Context, clusterId string) {
	if !clusterIDPattern.MatchString(clusterId) {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid cluster_id format: %q", clusterId)})
		return
	}

	filterExpr := fmt.Sprintf("cluster_id = '%s'", clusterId)
	reportID, clusters, err := h.rightsizingSrv.ListLatestClusterUtilization(c.Request.Context(), filterExpr)
	if err != nil {
		zap.S().Named("rightsizing_handler").Errorw("failed to get latest cluster utilization", "cluster_id", clusterId, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if reportID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "no completed rightsizing report found"})
		return
	}
	if len(clusters) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found in report"})
		return
	}
	c.JSON(http.StatusOK, v1.RightsizingClusterResponse{
		ReportId: reportID,
		Cluster:  v1.NewRightsizingClusterUtilizationFromModel(clusters[0]),
	})
}

// GetRightsizingReportCluster returns utilization for a specific cluster from a specific report.
// (GET /rightsizing/{report_id}/clusters/{cluster_id})
func (h *Handler) GetRightsizingReportCluster(c *gin.Context, reportId string, clusterId string) {
	if !clusterIDPattern.MatchString(clusterId) {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid cluster_id format: %q", clusterId)})
		return
	}
	if _, err := h.rightsizingSrv.GetReport(c.Request.Context(), reportId); err != nil {
		if srvErrors.IsResourceNotFoundError(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		zap.S().Named("rightsizing_handler").Errorw("failed to get report", "report_id", reportId, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filterExpr := fmt.Sprintf("cluster_id = '%s'", clusterId)
	clusters, err := h.rightsizingSrv.ListClusterUtilization(c.Request.Context(), reportId, filterExpr)
	if err != nil {
		zap.S().Named("rightsizing_handler").Errorw("failed to get cluster utilization", "report_id", reportId, "cluster_id", clusterId, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(clusters) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found in report"})
		return
	}
	c.JSON(http.StatusOK, v1.RightsizingClusterResponse{
		ReportId: reportId,
		Cluster:  v1.NewRightsizingClusterUtilizationFromModel(clusters[0]),
	})
}
